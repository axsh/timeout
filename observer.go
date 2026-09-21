package timeout

import "time"

// EventType names a policy-engine notification.
type EventType string

const (
	EventStarted             EventType = "execution_started"
	EventHeartbeat           EventType = "heartbeat_received"
	EventStatus              EventType = "status_changed"
	EventProgress            EventType = "progress_received"
	EventMeaningfulProgress  EventType = "meaningful_progress_confirmed"
	EventUnitStarted         EventType = "unit_started"
	EventUnitEnded           EventType = "unit_ended"
	EventTimeout             EventType = "timeout_detected"
	EventTerminationRequested EventType = "termination_requested"
	EventFinished            EventType = "execution_finished"
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

type asyncObserver struct {
	inner Observer
	ch    chan Event
}

func newAsyncObserver(inner Observer) *asyncObserver {
	if inner == nil {
		return nil
	}
	a := &asyncObserver{
		inner: inner,
		ch:    make(chan Event, 64),
	}
	go a.loop()
	return a
}

func (a *asyncObserver) loop() {
	for ev := range a.ch {
		a.inner.OnEvent(ev)
	}
}

func (a *asyncObserver) emit(ev Event) {
	if a == nil {
		return
	}
	select {
	case a.ch <- ev:
	default:
		// Drop on backpressure so the monitor is never blocked.
	}
}

func (a *asyncObserver) close() {
	if a == nil {
		return
	}
	close(a.ch)
}
