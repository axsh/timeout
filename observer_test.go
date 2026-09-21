package timeout

import (
	"sync"
	"testing"
	"time"
)

type recordingObserver struct {
	mu     sync.Mutex
	events []Event
	block  chan struct{}
}

func (r *recordingObserver) OnEvent(ev Event) {
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
}

func TestObserverDropNewest(t *testing.T) {
	block := make(chan struct{})
	obs := &recordingObserver{block: block}
	a := newAsyncObserver(obs, 1, DropNewest)
	defer func() {
		close(block)
		a.close()
	}()

	// Fill queue and cause drops while observer is blocked.
	a.emit(Event{Type: EventHeartbeat, Timestamp: time.Now()})
	for i := 0; i < 20; i++ {
		a.emit(Event{Type: EventHeartbeat, Timestamp: time.Now()})
	}
	if a.EventsDropped() == 0 {
		t.Fatal("expected drops")
	}
}

func TestObserverDropOldest(t *testing.T) {
	block := make(chan struct{})
	obs := &recordingObserver{block: block}
	a := newAsyncObserver(obs, 1, DropOldest)
	defer func() {
		close(block)
		a.close()
	}()

	a.emit(Event{Type: EventStarted, Timestamp: time.Now()})
	a.emit(Event{Type: EventFinished, Timestamp: time.Now()})
	// DropOldest should accept the newest without necessarily incrementing dropped
	// when replace succeeds; either way emit must not block.
	done := make(chan struct{})
	go func() {
		a.emit(Event{Type: EventHeartbeat, Timestamp: time.Now()})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit blocked")
	}
}

func TestObserverQueueDefault(t *testing.T) {
	a := newAsyncObserver(&recordingObserver{}, 0, DropNewest)
	defer a.close()
	if cap(a.ch) != defaultObserverQueue {
		t.Fatalf("cap=%d", cap(a.ch))
	}
}
