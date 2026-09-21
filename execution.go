package timeout

import (
	"context"
	"sync"
	"time"
)

// Execution is the supervised run handle passed to Func.
type Execution interface {
	Context() context.Context
	Heartbeat()
	Status(message string)
	Progress(progress Progress)
	ProgressTo(current, total int64)
	BeginUnit(name string) Unit
	Unit(name string, fn func(context.Context) error) error
	Snapshot() Snapshot
}

type execution struct {
	mu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	clock  Clock
	policy Policy

	startedAt      time.Time
	lastActivityAt time.Time
	lastProgressAt time.Time
	statusMsg      string
	progress       ProgressSnapshot
	series         progressSeries

	units      map[uint64]*unitState
	nextUnitID uint64

	state        State
	timedOut     bool
	kind         TimeoutKind
	timeoutUnit  *UnitSnapshot
	finishedOnce sync.Once
	done         chan struct{}

	observer *asyncObserver
}

func runExecution(parent context.Context, cfg Config, fn Func) Result {
	clock := cfg.clock
	if clock == nil {
		clock = realClock{}
	}

	ctx, cancel := context.WithCancel(parent)
	start := clock.Now()
	obs := newAsyncObserver(cfg.observer)

	exec := &execution{
		ctx:            ctx,
		cancel:         cancel,
		clock:          clock,
		policy:         cfg.policy,
		startedAt:      start,
		lastActivityAt: start,
		lastProgressAt: start,
		units:          make(map[uint64]*unitState),
		state:          StateRunning,
		done:           make(chan struct{}),
		observer:       obs,
	}

	exec.emit(EventStarted)

	go exec.monitor()

	var fnErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				fnErr = errString("timeout: panic in Func")
			}
		}()
		fnErr = fn(exec)
	}()

	result := exec.finish(parent, fnErr)
	obs.close()
	return result
}

func (e *execution) Context() context.Context { return e.ctx }

func (e *execution) Heartbeat() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timedOut || e.state != StateRunning {
		return
	}
	now := e.clock.Now()
	e.lastActivityAt = now
	e.emitLocked(EventHeartbeat, now)
}

func (e *execution) Status(message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timedOut || e.state != StateRunning {
		return
	}
	now := e.clock.Now()
	e.statusMsg = message
	e.lastActivityAt = now
	e.emitLocked(EventStatus, now)
}

func (e *execution) Progress(progress Progress) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timedOut || e.state != StateRunning {
		return
	}
	if progress.Total > 0 && progress.Current < 0 {
		return
	}
	now := e.clock.Now()
	idle, stall, next := classifyProgress(e.series, progress)
	e.series = next
	e.progress = progressSnapshotFrom(progress, now)
	if idle {
		e.lastActivityAt = now
	}
	ev := EventProgress
	if stall {
		e.lastProgressAt = now
		ev = EventMeaningfulProgress
	}
	e.emitLocked(ev, now)
}

func (e *execution) ProgressTo(current, total int64) {
	e.Progress(Progress{Current: current, Total: total})
}

func (e *execution) BeginUnit(name string) Unit {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.clock.Now()
	e.nextUnitID++
	id := e.nextUnitID
	e.units[id] = &unitState{id: id, name: name, startedAt: now}
	e.emitLocked(EventUnitStarted, now)
	return &unitHandle{exec: e, id: id, name: name}
}

func (e *execution) endUnit(id uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	u, ok := e.units[id]
	if !ok || u.ended {
		return
	}
	u.ended = true
	e.emitLocked(EventUnitEnded, e.clock.Now())
}

func (e *execution) Unit(name string, fn func(context.Context) error) error {
	u := e.BeginUnit(name)
	defer u.End()
	if fn == nil {
		return nil
	}
	return fn(e.Context())
}

func (e *execution) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked(e.clock.Now())
}

func (e *execution) snapshotLocked(now time.Time) Snapshot {
	units := make([]UnitSnapshot, 0, len(e.units))
	for _, u := range e.units {
		elapsed := now.Sub(u.startedAt)
		if u.ended {
			// Elapsed frozen at end is not tracked separately; use now for active only.
			elapsed = now.Sub(u.startedAt)
		}
		units = append(units, UnitSnapshot{
			ID:        u.id,
			Name:      u.name,
			StartedAt: u.startedAt,
			Elapsed:   elapsed,
			Limit:     e.policy.Unit,
			Active:    !u.ended,
		})
	}
	return Snapshot{
		State:          e.state,
		StartedAt:      e.startedAt,
		Elapsed:        now.Sub(e.startedAt),
		LastActivityAt: e.lastActivityAt,
		IdleFor:        now.Sub(e.lastActivityAt),
		LastProgressAt: e.lastProgressAt,
		StallFor:       now.Sub(e.lastProgressAt),
		Status:         e.statusMsg,
		Progress:       e.progress,
		Units:          units,
	}
}

func (e *execution) monitor() {
	defer close(e.done)

	var ticker *time.Ticker
	var tickerC <-chan time.Time
	if e.clock.Watcher() == nil {
		ticker = time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	for {
		if e.tryTimeout() {
			return
		}
		select {
		case <-e.ctx.Done():
			// Parent cancel or timeout cancel — exit monitor.
			return
		case <-e.clock.Watcher():
			continue
		case <-tickerC:
			continue
		}
	}
}

func (e *execution) tryTimeout() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timedOut || e.state != StateRunning {
		return e.timedOut
	}
	now := e.clock.Now()
	kind, unitSnap := e.evaluateLocked(now)
	if kind == KindNone {
		return false
	}
	e.timedOut = true
	e.kind = kind
	e.timeoutUnit = unitSnap
	e.emitLocked(EventTimeout, now)
	e.emitLocked(EventTerminationRequested, now)
	e.cancel()
	return true
}

// evaluateLocked applies Hard > Unit > Stall > Idle.
func (e *execution) evaluateLocked(now time.Time) (TimeoutKind, *UnitSnapshot) {
	if e.policy.Hard > 0 && now.Sub(e.startedAt) >= e.policy.Hard {
		return KindHard, nil
	}
	if e.policy.Unit > 0 {
		for _, u := range e.units {
			if u.ended {
				continue
			}
			if now.Sub(u.startedAt) >= e.policy.Unit {
				snap := UnitSnapshot{
					ID:        u.id,
					Name:      u.name,
					StartedAt: u.startedAt,
					Elapsed:   now.Sub(u.startedAt),
					Limit:     e.policy.Unit,
					Active:    true,
				}
				return KindUnit, &snap
			}
		}
	}
	if e.policy.Stall > 0 && now.Sub(e.lastProgressAt) >= e.policy.Stall {
		return KindStall, nil
	}
	if e.policy.Idle > 0 && now.Sub(e.lastActivityAt) >= e.policy.Idle {
		return KindIdle, nil
	}
	return KindNone, nil
}

func (e *execution) finish(parent context.Context, fnErr error) Result {
	var result Result
	e.finishedOnce.Do(func() {
		// Stop monitor by canceling if still running without timeout.
		e.mu.Lock()
		timedOut := e.timedOut
		kind := e.kind
		unitSnap := e.timeoutUnit
		started := e.startedAt
		e.mu.Unlock()

		if !timedOut {
			e.cancel()
		}
		<-e.done

		now := e.clock.Now()
		e.mu.Lock()
		e.state = StateFinished
		snap := e.snapshotLocked(now)
		timedOut = e.timedOut
		kind = e.kind
		unitSnap = e.timeoutUnit
		e.mu.Unlock()

		result = Result{
			Snapshot:    snap,
			StartedAt:   started,
			FinishedAt:  now,
			Elapsed:     now.Sub(started),
			TimeoutUnit: unitSnap,
		}

		switch {
		case timedOut:
			result.Status = StatusTimeout
			result.Kind = kind
		case parent.Err() != nil:
			result.Status = StatusCanceled
			result.Kind = KindNone
			result.Err = parent.Err()
		case fnErr != nil:
			result.Status = StatusFailed
			result.Kind = KindNone
			result.Err = fnErr
		default:
			result.Status = StatusSucceeded
			result.Kind = KindNone
		}

		e.mu.Lock()
		e.emitLocked(EventFinished, now)
		e.mu.Unlock()
	})
	return result
}

func (e *execution) emit(typ EventType) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.emitLocked(typ, e.clock.Now())
}

func (e *execution) emitLocked(typ EventType, now time.Time) {
	if e.observer == nil {
		return
	}
	e.observer.emit(Event{
		Type:      typ,
		Timestamp: now,
		Snapshot:  e.snapshotLocked(now),
	})
}
