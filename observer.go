package timeout

import (
	"sync/atomic"
	"time"
)

// EventType names a policy-engine notification.
type EventType string

const (
	EventStarted              EventType = "execution_started"
	EventHeartbeat            EventType = "heartbeat_received"
	EventStatus               EventType = "status_changed"
	EventProgress             EventType = "progress_received"
	EventMeaningfulProgress   EventType = "meaningful_progress_confirmed"
	EventUnitStarted          EventType = "unit_started"
	EventUnitEnded            EventType = "unit_ended"
	EventTimeout              EventType = "timeout_detected"
	EventTerminationRequested EventType = "termination_requested"
	EventFinished             EventType = "execution_finished"
)

// Event is delivered to observers.
type Event struct {
	Type      EventType
	Timestamp time.Time
	Snapshot  Snapshot
}

// Observer receives engine events. Implementations must not block the monitor.
type Observer interface {
	OnEvent(Event)
}

// DropPolicy controls observer queue overflow behavior.
type DropPolicy int

const (
	// DropNewest discards incoming events when the queue is full (default).
	DropNewest DropPolicy = iota
	// DropOldest discards the oldest queued event to make room for a new one.
	DropOldest
)

const defaultObserverQueue = 64

type asyncObserver struct {
	inner      Observer
	ch         chan Event
	dropPolicy DropPolicy
	dropped    atomic.Uint64
}

func newAsyncObserver(inner Observer, size int, policy DropPolicy) *asyncObserver {
	if inner == nil {
		return nil
	}
	if size <= 0 {
		size = defaultObserverQueue
	}
	a := &asyncObserver{
		inner:      inner,
		ch:         make(chan Event, size),
		dropPolicy: policy,
	}
	go a.loop()
	return a
}

func (a *asyncObserver) loop() {
	for ev := range a.ch {
		a.inner.OnEvent(ev)
	}
}

func (a *asyncObserver) EventsDropped() uint64 {
	if a == nil {
		return 0
	}
	return a.dropped.Load()
}

func (a *asyncObserver) emit(ev Event) {
	if a == nil {
		return
	}
	select {
	case a.ch <- ev:
		return
	default:
	}
	if a.dropPolicy == DropOldest {
		select {
		case <-a.ch:
		default:
		}
		select {
		case a.ch <- ev:
			return
		default:
			a.dropped.Add(1)
			return
		}
	}
	a.dropped.Add(1)
}

func (a *asyncObserver) close() {
	if a == nil {
		return
	}
	close(a.ch)
}
