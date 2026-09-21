package timeout

import (
	"context"
	"time"
)

// Config is an immutable execution configuration produced by New.
// Each Run creates a fresh Execution; Config may be reused concurrently.
type Config struct {
	policy       Policy
	clock        Clock
	observer     Observer
	observerSize int
	observerDrop DropPolicy
	err          error
}

// New builds a Config from functional options.
// All time policies default to disabled (0).
func New(options ...Option) Config {
	c := Config{
		clock: realClock{},
	}
	for _, opt := range options {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// Policy returns a copy of the configured policy.
func (c Config) Policy() Policy {
	return c.policy
}

// Apply returns a copy of c with additional options applied.
func (c Config) Apply(options ...Option) Config {
	for _, opt := range options {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// Func is the user work function supervised by the policy engine.
type Func func(Execution) error

// Run starts a new supervised execution.
func Run(ctx context.Context, cfg Config, fn Func) Result {
	return cfg.Run(ctx, fn)
}

// Run starts a new supervised execution using this Config.
func (c Config) Run(ctx context.Context, fn Func) Result {
	if c.err != nil {
		now := time.Now()
		if c.clock != nil {
			now = c.clock.Now()
		}
		return Result{
			Status:     StatusInternalError,
			Kind:       KindNone,
			Err:        c.err,
			StartedAt:  now,
			FinishedAt: now,
		}
	}
	if fn == nil {
		now := c.clock.Now()
		return Result{
			Status:     StatusInternalError,
			Kind:       KindNone,
			Err:        errNilFunc,
			StartedAt:  now,
			FinishedAt: now,
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return runExecution(ctx, c, fn)
}

var errNilFunc = errString("timeout: Func must not be nil")

type errString string

func (e errString) Error() string { return string(e) }
