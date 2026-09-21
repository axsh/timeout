package timeout

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/axsh/timeout/protocol"
)

// ProbeOnFailure controls command-probe failure handling.
type ProbeOnFailure string

const (
	ProbeOnFailureIgnore    ProbeOnFailure = "ignore"
	ProbeOnFailureWarn      ProbeOnFailure = "warn"
	ProbeOnFailureFailProbe ProbeOnFailure = "fail-probe"
)

// CommandProbe periodically runs an external command and applies NDJSON signals.
type CommandProbe struct {
	Command     string
	Args        []string
	Interval    time.Duration
	OnFailure   ProbeOnFailure
	EmptySignal string // "", "heartbeat", or "progress"
	Verbose     bool
}

// ProbeDiagnostic accumulates probe diagnostics for Result JSON.
type ProbeDiagnostic struct {
	mu             sync.Mutex
	LastError      string
	SkippedOverlap int
	Failures       int
}

func (d *ProbeDiagnostic) Snapshot() map[string]any {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return map[string]any{
		"lastError":      d.LastError,
		"skippedOverlap": d.SkippedOverlap,
		"failures":       d.Failures,
	}
}

func (d *ProbeDiagnostic) setError(msg string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.LastError = msg
	d.Failures++
	d.mu.Unlock()
}

func (d *ProbeDiagnostic) skip() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.SkippedOverlap++
	d.mu.Unlock()
}

// StartCommandProbe runs until ctx is done.
func StartCommandProbe(
	ctx context.Context,
	p CommandProbe,
	apply func(protocol.Envelope),
	onFatal func(error),
	diag *ProbeDiagnostic,
) {
	interval := p.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	onFailure := p.OnFailure
	if onFailure == "" {
		onFailure = ProbeOnFailureWarn
	}

	var running atomic.Bool
	t := time.NewTicker(interval)
	defer t.Stop()

	runOnce := func() {
		if !running.CompareAndSwap(false, true) {
			diag.skip()
			return
		}
		defer running.Store(false)

		cmd := exec.CommandContext(ctx, p.Command, p.Args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if ctx.Err() != nil {
			return
		}
		if p.Verbose && stderr.Len() > 0 {
			fmt.Fprintf(os.Stderr, "timeoutx: probe: %s", stderr.String())
		}
		if err != nil {
			msg := err.Error()
			diag.setError(msg)
			switch onFailure {
			case ProbeOnFailureIgnore:
				return
			case ProbeOnFailureFailProbe:
				if onFatal != nil {
					onFatal(ProbeFailure(err))
				}
				return
			default: // warn
				fmt.Fprintf(os.Stderr, "timeoutx: probe warning: %v\n", err)
				return
			}
		}

		data := bytes.TrimSpace(stdout.Bytes())
		if len(data) == 0 {
			switch p.EmptySignal {
			case "heartbeat":
				apply(protocol.Envelope{V: 1, Type: "heartbeat"})
			case "progress":
				apply(protocol.Envelope{V: 1, Type: "progress", Message: "probe"})
			}
			return
		}
		_ = protocol.ReadLoop(bytes.NewReader(append(data, '\n')), func(env protocol.Envelope) error {
			switch env.Type {
			case "heartbeat", "status", "progress":
				apply(env)
			default:
				fmt.Fprintf(os.Stderr, "timeoutx: probe: ignored type %q\n", env.Type)
			}
			return nil
		}, func(err error) {
			diag.setError(err.Error())
			fmt.Fprintf(os.Stderr, "timeoutx: probe protocol: %v\n", err)
		})
	}

	// First run immediately, then on interval.
	runOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			runOnce()
		}
	}
}
