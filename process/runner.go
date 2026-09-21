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
	"sync/atomic"
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
	Probes             []timeout.CommandProbe
	EventsPath         string
	EventsFD           int // -1 unset
	TimeoutExit        int
	SignalExit         bool
	KilledBySIGKILL    *bool // optional out
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
	if req.TimeoutExit == 0 {
		req.TimeoutExit = 124
	}
	if req.EventsFD == 0 {
		req.EventsFD = -1
	}
	if req.WarnNoPolicy {
		p := req.Config.Policy()
		if p.Hard == 0 && p.Idle == 0 && p.Stall == 0 && p.Unit == 0 {
			fmt.Fprintln(os.Stderr, "timeoutx: warning: all time policies are disabled")
		}
	}

	ew, err := OpenEventWriter(req.EventsPath, req.EventsFD)
	if err != nil {
		return timeout.Result{Status: timeout.StatusInternalError, Err: err}, 125
	}
	cfg := req.Config
	if ew != nil || req.ProgressFormatJSON {
		if ew == nil && req.ProgressFormatJSON {
			ew, _ = OpenEventWriter("-", -1)
		}
		cfg = cfg.Apply(timeout.WithObserver(eventObserver{w: ew}))
	}

	exitCode := 0
	var cmdErr error
	var killedBySIGKILL atomic.Bool
	var probeFatal atomic.Value // error
	probeDiag := &timeout.ProbeDiagnostic{}

	result := timeout.Run(req.Ctx, cfg, func(execH timeout.Execution) error {
		cmd := exec.CommandContext(context.Background(), req.Command, req.Args...)
		cmd.Dir = req.Dir
		cmd.Env = append(os.Environ(), req.Env...)

		controlR, controlW, err := os.Pipe()
		if err != nil {
			return err
		}
		defer controlR.Close()

		_, fdNum, err := prepareControlFD(cmd, controlW)
		if err != nil {
			controlW.Close()
			return err
		}
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

		job, jobErr := attachJob(cmd)
		if jobErr != nil {
			fmt.Fprintf(os.Stderr, "timeoutx: warning: job object unavailable: %v\n", jobErr)
		}
		defer closeJob(job)

		unitHandles := &unitHandleMap{m: make(map[uint64]timeout.Unit)}
		applyEnv := func(env protocol.Envelope) {
			applyEnvelope(execH, env, unitHandles)
		}

		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = protocol.ReadLoop(controlR, func(env protocol.Envelope) error {
				applyEnv(env)
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

		probeCtx, probeStop := context.WithCancel(context.Background())
		defer probeStop()
		for _, p := range req.Probes {
			p := p
			go timeout.StartCommandProbe(probeCtx, p, applyEnv, func(err error) {
				probeFatal.Store(err)
				probeStop()
				execH.Context() // touch
				// Cancel by killing child so Func can return.
				_ = terminateProcessGroup(cmd.Process, req.KillAfter, job)
			}, probeDiag)
		}

		var fileStop context.CancelFunc
		if req.ProgressFile != "" {
			var fileCtx context.Context
			fileCtx, fileStop = context.WithCancel(context.Background())
			go watchProgressFile(fileCtx, req.ProgressFile, execH)
		}

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		select {
		case <-execH.Context().Done():
			_ = terminateProcessGroup(cmd.Process, req.KillAfter, job)
			killedBySIGKILL.Store(true)
			cmdErr = <-waitCh
		case cmdErr = <-waitCh:
		}

		if fileStop != nil {
			fileStop()
		}
		probeStop()
		_ = controlR.Close()
		wg.Wait()

		if v := probeFatal.Load(); v != nil {
			return v.(error)
		}
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

	sigKill := killedBySIGKILL.Load()
	if req.KilledBySIGKILL != nil {
		*req.KilledBySIGKILL = sigKill
	}
	code := mapExitCode(result, exitCode, cmdErr, req.TimeoutExit, req.SignalExit, sigKill)
	if result.Status == timeout.StatusTimeout {
		printTimeoutDiag(result)
	}
	if req.ResultPath != "" {
		_ = writeResultJSON(req.ResultPath, result, code, probeDiag)
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

func mapExitCode(result timeout.Result, childCode int, cmdErr error, timeoutExit int, signalExit, killedBySIGKILL bool) int {
	switch result.Status {
	case timeout.StatusTimeout:
		if signalExit && killedBySIGKILL {
			return 137
		}
		if timeoutExit == 0 {
			return 124
		}
		return timeoutExit
	case timeout.StatusInternalError:
		return 125
	case timeout.StatusFailed:
		if result.Kind == timeout.KindProbe {
			return 125
		}
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
	SchemaVersion int            `json:"schemaVersion"`
	Status        string         `json:"status"`
	Kind          string         `json:"kind"`
	ExitCode      int            `json:"exitCode"`
	Elapsed       string         `json:"elapsed"`
	Idle          string         `json:"idle"`
	Stall         string         `json:"stall"`
	StatusMessage string         `json:"statusMessage,omitempty"`
	EventsDropped uint64         `json:"eventsDropped,omitempty"`
	Progress      *progressJSON  `json:"progress,omitempty"`
	Timeout       *timeoutJSON   `json:"timeout,omitempty"`
	Probes        map[string]any `json:"probes,omitempty"`
}

type progressJSON struct {
	Stage     string          `json:"stage,omitempty"`
	Current   int64           `json:"current"`
	Total     int64           `json:"total"`
	Percent   float64         `json:"percent"`
	Message   string          `json:"message,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
}

type timeoutJSON struct {
	Limit      string `json:"limit,omitempty"`
	DetectedAt string `json:"detectedAt,omitempty"`
}

func writeResultJSON(path string, result timeout.Result, exitCode int, diag *timeout.ProbeDiagnostic) error {
	rj := resultJSON{
		SchemaVersion: 1,
		Status:        string(result.Status),
		Kind:          string(result.Kind),
		ExitCode:      exitCode,
		Elapsed:       result.Elapsed.String(),
		Idle:          result.Snapshot.IdleFor.String(),
		Stall:         result.Snapshot.StallFor.String(),
		StatusMessage: result.Snapshot.Status,
		EventsDropped: result.EventsDropped,
	}
	if !result.Snapshot.Progress.UpdatedAt.IsZero() || result.Snapshot.Progress.Message != "" || result.Snapshot.Progress.Total > 0 {
		pj := &progressJSON{
			Stage:     result.Snapshot.Progress.Stage,
			Current:   result.Snapshot.Progress.Current,
			Total:     result.Snapshot.Progress.Total,
			Percent:   result.Snapshot.Progress.Percent,
			Message:   result.Snapshot.Progress.Message,
			UpdatedAt: result.Snapshot.Progress.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if raw, ok := timeout.MarshalDetailsJSON(result.Snapshot.Progress.Details); ok && raw != nil {
			pj.Details = raw
		} else if !ok {
			fmt.Fprintln(os.Stderr, "timeoutx: details_marshal_error")
		}
		rj.Progress = pj
	}
	if result.Status == timeout.StatusTimeout {
		rj.Timeout = &timeoutJSON{
			DetectedAt: result.FinishedAt.UTC().Format(time.RFC3339),
		}
	}
	if diag != nil {
		rj.Probes = diag.Snapshot()
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
