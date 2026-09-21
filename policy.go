package timeout

import (
	"fmt"
	"time"
)

// Policy holds duration limits for an execution.
// A duration of 0 disables that policy. Negative durations are invalid.
type Policy struct {
	Hard      time.Duration
	Idle      time.Duration
	Stall     time.Duration
	Unit      time.Duration
	KillAfter time.Duration
}

// Option configures a Config at New time.
type Option func(*Config)

// Hard sets the hard timeout (total elapsed since start).
func Hard(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			c.err = fmt.Errorf("timeout: Hard duration must not be negative: %v", d)
			return
		}
		c.policy.Hard = d
	}
}

// Idle sets the idle timeout (elapsed since last activity).
func Idle(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			c.err = fmt.Errorf("timeout: Idle duration must not be negative: %v", d)
			return
		}
		c.policy.Idle = d
	}
}

// Stall sets the stall timeout (elapsed since last meaningful progress).
func Stall(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			c.err = fmt.Errorf("timeout: Stall duration must not be negative: %v", d)
			return
		}
		c.policy.Stall = d
	}
}

// UnitLimit sets the per-unit timeout.
func UnitLimit(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			c.err = fmt.Errorf("timeout: Unit duration must not be negative: %v", d)
			return
		}
		c.policy.Unit = d
	}
}

// KillAfter sets the grace period after SIGTERM before SIGKILL (CLI/process runner).
func KillAfter(d time.Duration) Option {
	return func(c *Config) {
		if d < 0 {
			c.err = fmt.Errorf("timeout: KillAfter duration must not be negative: %v", d)
			return
		}
		c.policy.KillAfter = d
	}
}

// WithClock injects a custom clock (for tests).
func WithClock(clock Clock) Option {
	return func(c *Config) {
		if clock != nil {
			c.clock = clock
		}
	}
}

// WithObserver registers an event observer.
func WithObserver(o Observer) Option {
	return func(c *Config) {
		c.observer = o
	}
}
