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
type ManualClock struct {
	mu      sync.Mutex
	now     time.Time
	watcher chan struct{}
}

// NewManualClock returns a ManualClock starting at start.
func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{
		now:     start,
		watcher: make(chan struct{}),
	}
}

// Now returns the current fake time.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Watcher returns a channel closed when Advance is called.
func (c *ManualClock) Watcher() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.watcher
}

// Advance moves the clock forward and notifies watchers.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	old := c.watcher
	c.watcher = make(chan struct{})
	c.mu.Unlock()
	close(old)
}
