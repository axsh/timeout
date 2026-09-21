package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/axsh/timeout"
	"github.com/axsh/timeout/protocol"
)

// RunRequest configures a supervised child process.
type RunRequest struct {
	Ctx                context.Context
	Config             timeout.Config
	Command            string
	Args               []string
	Env                []string
	Dir                string
	KillAfter          time.Duration
	HeartbeatOnOutput  bool
	ProgressOnOutput   bool
	ProgressFile       string
	ResultPath         string
	ShowProgress       bool
	ProgressFormatJSON bool
	WarnNoPolicy       bool
}

// Runner starts and supervises OS processes with the policy engine.
type Runner struct{}

// Run starts the child command under the timeout policy engine.
func (Runner) Run(req RunRequest) (timeout.Result, int) {
	if req.Ctx == nil {
		req.Ctx = context.Background()
	}
	if req.KillAfter <= 0 {
		req.KillAfter = 10 * time.Second
	}
	if req.WarnNoPolicy {
		p := req.Config.Policy()
		if p.Hard == 0 && p.Idle == 0 && p.Stall == 0 && p.Unit == 0 {
			fmt.Fprintln(os.Stderr, "timeoutx: warning: all time policies are disabled")
		}
	}

	exitCode := 0
	var cmdErr error

	result := timeout.Run(req.Ctx, req.Config, func(execH timeout.Execution) error {
		cmd := exec.CommandContext(context.Background(), req.Command, req.Args...)
		cmd.Dir = req.Dir
		cmd.Env = append(os.Environ(), req.Env...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		controlR, controlW, err := os.Pipe()
		if err != nil {
			return err
		}
		defer controlR.Close()

		extraFiles, fdNum, err := prepareControlFD(cmd, controlW)
		if err != nil {
			controlW.Close()
			return err
		}
		_ = extraFiles
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("TIMEOUTX_FD=%d", fdNum),
			"TIMEOUTX_PROTOCOL=1",
		)

		stdoutR, stdoutW, err := os.Pipe()
		if err != nil {
			controlW.Close()
			return err
		}
		stderrR, stderrW, err := os.Pipe()
		if err != nil {
			controlW.Close()
			stdoutR.Close()
			stdoutW.Close()
			return err
		}
		cmd.Stdout = stdoutW
		cmd.Stderr = stderrW

		if err := setProcessGroup(cmd); err != nil {
			controlW.Close()
			stdoutW.Close()
			stderrW.Close()
			return err
		}

		if err := cmd.Start(); err != nil {
			controlW.Close()
			stdoutW.Close()
			stderrW.Close()
			cmdErr = err
			return err
		}
		controlW.Close()
		stdoutW.Close()
		stderrW.Close()

		var wg sync.WaitGroup
		wg.Add(3)
		unitHandles := &unitHandleMap{m: make(map[uint64]timeout.Unit)}
		go func() {
			defer wg.Done()
			_ = protocol.ReadLoop(controlR, func(env protocol.Envelope) error {
				applyEnvelope(execH, env, unitHandles)
				return nil
			}, func(err error) {
				fmt.Fprintf(os.Stderr, "timeoutx: protocol: %v\n", err)
			})
		}()
		go func() {
			defer wg.Done()
			pipeOutput(stdoutR, os.Stdout, execH, req.HeartbeatOnOutput, req.ProgressOnOutput)
		}()
		go func() {
			defer wg.Done()
			pipeOutput(stderrR, os.Stderr, execH, req.HeartbeatOnOutput, req.ProgressOnOutput)
		}()

		var probeStop context.CancelFunc
		if req.ProgressFile != "" {
			var probeCtx context.Context
			probeCtx, probeStop = context.WithCancel(context.Background())
			go watchProgressFile(probeCtx, req.ProgressFile, execH)
		}

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		select {
		case <-execH.Context().Done():
			_ = terminateProcessGroup(cmd.Process, req.KillAfter)
			cmdErr = <-waitCh
		case cmdErr = <-waitCh:
		}

		if probeStop != nil {
			probeStop()
		}
		_ = controlR.Close()
		wg.Wait()

		if cmdErr != nil {
			if ee, ok := cmdErr.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
				return nil
			}
			return cmdErr
		}
		exitCode = 0
		return nil
	})

	code := mapExitCode(result, exitCode, cmdErr)
	if result.Status == timeout.StatusTimeout {
		printTimeoutDiag(result)
	}
	if req.ResultPath != "" {
		_ = writeResultJSON(req.ResultPath, result)
	}
	return result, code
}

type unitHandleMap struct {
	sync.Mutex
	m map[uint64]timeout.Unit
}

func applyEnvelope(execH timeout.Execution, env protocol.Envelope, units *unitHandleMap) {
	switch env.Type {
	case "heartbeat":
		execH.Heartbeat()
	case "status":
		execH.Status(env.Message)
	case "progress":
		p := timeout.Progress{Stage: env.Stage, Message: env.Message}
		if env.Current != nil {
			p.Current = *env.Current
		}
		if env.Total != nil {
			p.Total = *env.Total
		}
		if len(env.Details) > 0 {
			var details any
			_ = json.Unmarshal(env.Details, &details)
			p.Details = details
		}
		execH.Progress(p)
	case "unit_begin":
		u := execH.BeginUnit(env.Name)
		units.Lock()
		units.m[env.ID] = u
		units.Unlock()
	case "unit_end":
		units.Lock()
		u := units.m[env.ID]
		delete(units.m, env.ID)
		units.Unlock()
		if u != nil {
			u.End()
		}
	}
}

func pipeOutput(r io.Reader, w io.Writer, execH timeout.Execution, hb, prog bool) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if hb {
				execH.Heartbeat()
			}
			if prog {
				execH.Progress(timeout.Progress{Message: "output"})
			}
		}
		if err != nil {
			return
		}
	}
}

func watchProgressFile(ctx context.Context, path string, execH timeout.Execution) {
	var lastSize int64 = -1
	var lastMod time.Time
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			if lastSize >= 0 && (fi.Size() != lastSize || !fi.ModTime().Equal(lastMod)) {
				execH.Progress(timeout.Progress{
					Stage:   "file-probe",
					Message: path,
				})
			}
			lastSize = fi.Size()
			lastMod = fi.ModTime()
		}
	}
}

func mapExitCode(result timeout.Result, childCode int, cmdErr error) int {
	switch result.Status {
	case timeout.StatusTimeout:
		return 124
	case timeout.StatusInternalError:
		return 125
	case timeout.StatusCanceled:
		return 130
	}
	if cmdErr != nil {
		if ee, ok := cmdErr.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		if errors.Is(cmdErr, exec.ErrNotFound) {
			return 127
		}
		var pe *fs.PathError
		if errors.As(cmdErr, &pe) {
			return 127
		}
		return 126
	}
	return childCode
}

func printTimeoutDiag(result timeout.Result) {
	fmt.Fprintf(os.Stderr, "timeoutx: %s timeout\n", result.Kind)
	if result.Snapshot.Progress.Total > 0 {
		fmt.Fprintf(os.Stderr, "timeoutx: progress: %d / %d (%.0f%%)\n",
			result.Snapshot.Progress.Current,
			result.Snapshot.Progress.Total,
			result.Snapshot.Progress.Percent,
		)
	}
	if result.Snapshot.Progress.Message != "" {
		fmt.Fprintf(os.Stderr, "timeoutx: last activity: %q\n", result.Snapshot.Progress.Message)
	} else if result.Snapshot.Status != "" {
		fmt.Fprintf(os.Stderr, "timeoutx: last activity: %q\n", result.Snapshot.Status)
	}
}

type resultJSON struct {
	Status        string          `json:"status"`
	Kind          string          `json:"kind"`
	Elapsed       string          `json:"elapsed"`
	Idle          string          `json:"idle"`
	Stall         string          `json:"stall"`
	StatusMessage string          `json:"statusMessage,omitempty"`
	Progress      *progressJSON   `json:"progress,omitempty"`
	Timeout       *timeoutJSON    `json:"timeout,omitempty"`
}

type progressJSON struct {
	Stage     string  `json:"stage,omitempty"`
	Current   int64   `json:"current"`
	Total     int64   `json:"total"`
	Percent   float64 `json:"percent"`
	Message   string  `json:"message,omitempty"`
	UpdatedAt string  `json:"updatedAt,omitempty"`
}

type timeoutJSON struct {
	Limit      string `json:"limit,omitempty"`
	DetectedAt string `json:"detectedAt,omitempty"`
}

func writeResultJSON(path string, result timeout.Result) error {
	rj := resultJSON{
		Status:        string(result.Status),
		Kind:          string(result.Kind),
		Elapsed:       result.Elapsed.String(),
		Idle:          result.Snapshot.IdleFor.String(),
		Stall:         result.Snapshot.StallFor.String(),
		StatusMessage: result.Snapshot.Status,
	}
	if result.Snapshot.Progress.UpdatedAt.IsZero() == false || result.Snapshot.Progress.Message != "" || result.Snapshot.Progress.Total > 0 {
		rj.Progress = &progressJSON{
			Stage:     result.Snapshot.Progress.Stage,
			Current:   result.Snapshot.Progress.Current,
			Total:     result.Snapshot.Progress.Total,
			Percent:   result.Snapshot.Progress.Percent,
			Message:   result.Snapshot.Progress.Message,
			UpdatedAt: result.Snapshot.Progress.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	if result.Status == timeout.StatusTimeout {
		rj.Timeout = &timeoutJSON{
			DetectedAt: result.FinishedAt.UTC().Format(time.RFC3339),
		}
	}
	data, err := json.MarshalIndent(rj, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".timeoutx-result-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
