package timeout

import (
	"sync"
	"time"
)

// Clock provides the time source used by the policy engine.
// Tests may inject a ManualClock to advance time deterministically.
type Clock interface {
	Now() time.Time
	// Watcher returns a channel that receives when time may have advanced.
	// A nil channel means the engine should poll with a real ticker.
	Watcher() <-chan struct{}
}

type realClock struct{}

func (realClock) Now() time.Time           { return time.Now() }
func (realClock) Watcher() <-chan struct{} { return nil }

// ManualClock is a controllable clock for tests.
// Advance signals Watcher with a buffered notification so a monitor that has
// not entered select yet still observes the new time.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
	ch  chan struct{}
}

// NewManualClock returns a ManualClock starting at start.
func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{
		now: start,
		ch:  make(chan struct{}, 1),
	}
}

// Now returns the current fake time.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Watcher returns a channel that receives when Advance is called.
func (c *ManualClock) Watcher() <-chan struct{} {
	return c.ch
}

// Advance moves the clock forward and notifies watchers.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	select {
	case c.ch <- struct{}{}:
	default:
	}
}
