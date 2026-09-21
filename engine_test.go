package timeout

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestClassifyProgress(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		prev        progressSeries
		p           Progress
		wantIdle    bool
		wantStall   bool
	}{
		{
			name:      "qualitative",
			p:         Progress{Stage: "resolve", Message: "done"},
			wantIdle:  true,
			wantStall: true,
		},
		{
			name:      "first quantitative",
			p:         Progress{Stage: "import", Current: 1, Total: 10},
			wantIdle:  true,
			wantStall: true,
		},
		{
			name:      "current increased",
			prev:      progressSeries{stage: "import", total: 10, current: 1, hasQuant: true},
			p:         Progress{Stage: "import", Current: 2, Total: 10},
			wantIdle:  true,
			wantStall: true,
		},
		{
			name:      "current same",
			prev:      progressSeries{stage: "import", total: 10, current: 3, hasQuant: true},
			p:         Progress{Stage: "import", Current: 3, Total: 10},
			wantIdle:  true,
			wantStall: false,
		},
		{
			name:      "current decreased",
			prev:      progressSeries{stage: "import", total: 10, current: 5, hasQuant: true},
			p:         Progress{Stage: "import", Current: 4, Total: 10},
			wantIdle:  true,
			wantStall: false,
		},
		{
			name:      "stage changed",
			prev:      progressSeries{stage: "a", total: 10, current: 5, hasQuant: true},
			p:         Progress{Stage: "b", Current: 1, Total: 10},
			wantIdle:  true,
			wantStall: true,
		},
		{
			name:      "total changed",
			prev:      progressSeries{stage: "import", total: 10, current: 5, hasQuant: true},
			p:         Progress{Stage: "import", Current: 5, Total: 20},
			wantIdle:  true,
			wantStall: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idle, stall, _ := classifyProgress(tc.prev, tc.p)
			if idle != tc.wantIdle || stall != tc.wantStall {
				t.Fatalf("idle=%v stall=%v want idle=%v stall=%v", idle, stall, tc.wantIdle, tc.wantStall)
			}
		})
	}
}

func TestHardTimeout(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	resultCh := make(chan Result, 1)

	go func() {
		resultCh <- Run(context.Background(),
			New(Hard(5*time.Second), WithClock(clk)),
			func(exec Execution) error {
				close(started)
				<-exec.Context().Done()
				return nil
			},
		)
	}()

	<-started
	clk.Advance(5 * time.Second)
	res := <-resultCh
	if res.Status != StatusTimeout || res.Kind != KindHard {
		t.Fatalf("got status=%s kind=%s", res.Status, res.Kind)
	}
}

func TestIdleTimeout(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	resultCh := make(chan Result, 1)

	go func() {
		resultCh <- Run(context.Background(),
			New(Idle(2*time.Second), WithClock(clk)),
			func(exec Execution) error {
				close(started)
				<-exec.Context().Done()
				return nil
			},
		)
	}()

	<-started
	clk.Advance(2 * time.Second)
	res := <-resultCh
	if res.Status != StatusTimeout || res.Kind != KindIdle {
		t.Fatalf("got status=%s kind=%s", res.Status, res.Kind)
	}
}

func TestHeartbeatDoesNotClearStall(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	resultCh := make(chan Result, 1)

	go func() {
		resultCh <- Run(context.Background(),
			New(Stall(3*time.Second), WithClock(clk)),
			func(exec Execution) error {
				close(started)
				for {
					select {
					case <-exec.Context().Done():
						return nil
					case <-time.After(1 * time.Millisecond):
						exec.Heartbeat()
					}
				}
			},
		)
	}()

	<-started
	clk.Advance(3 * time.Second)
	res := <-resultCh
	if res.Status != StatusTimeout || res.Kind != KindStall {
		t.Fatalf("got status=%s kind=%s", res.Status, res.Kind)
	}
}

func TestSameProgressDoesNotClearStall(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	resultCh := make(chan Result, 1)

	go func() {
		resultCh <- Run(context.Background(),
			New(Stall(3*time.Second), WithClock(clk)),
			func(exec Execution) error {
				exec.Progress(Progress{Stage: "import", Current: 3, Total: 10})
				close(started)
				for {
					select {
					case <-exec.Context().Done():
						return nil
					case <-time.After(1 * time.Millisecond):
						exec.Progress(Progress{Stage: "import", Current: 3, Total: 10})
					}
				}
			},
		)
	}()

	<-started
	clk.Advance(3 * time.Second)
	res := <-resultCh
	if res.Status != StatusTimeout || res.Kind != KindStall {
		t.Fatalf("got status=%s kind=%s", res.Status, res.Kind)
	}
}

func TestProgressIncreaseClearsIdleAndStall(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	res := Run(context.Background(),
		New(Idle(10*time.Second), Stall(10*time.Second), WithClock(clk)),
		func(exec Execution) error {
			clk.Advance(4 * time.Second)
			exec.Progress(Progress{Stage: "import", Current: 1, Total: 10})
			snap := exec.Snapshot()
			if snap.IdleFor != 0 || snap.StallFor != 0 {
				t.Fatalf("expected idle/stall reset, idle=%v stall=%v", snap.IdleFor, snap.StallFor)
			}
			return nil
		},
	)
	if res.Status != StatusSucceeded {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestStatusClearsIdleOnly(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	res := Run(context.Background(),
		New(Idle(10*time.Second), Stall(10*time.Second), WithClock(clk)),
		func(exec Execution) error {
			clk.Advance(4 * time.Second)
			exec.Status("waiting")
			snap := exec.Snapshot()
			if snap.IdleFor != 0 {
				t.Fatalf("idle should reset, got %v", snap.IdleFor)
			}
			if snap.StallFor != 4*time.Second {
				t.Fatalf("stall should remain 4s, got %v", snap.StallFor)
			}
			return nil
		},
	)
	if res.Status != StatusSucceeded {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestUnitTimeout(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	resultCh := make(chan Result, 1)

	go func() {
		resultCh <- Run(context.Background(),
			New(UnitLimit(2*time.Second), WithClock(clk)),
			func(exec Execution) error {
				_ = exec.BeginUnit("download")
				close(started)
				<-exec.Context().Done()
				return nil
			},
		)
	}()

	<-started
	clk.Advance(2 * time.Second)
	res := <-resultCh
	if res.Status != StatusTimeout || res.Kind != KindUnit {
		t.Fatalf("got status=%s kind=%s", res.Status, res.Kind)
	}
	if res.TimeoutUnit == nil || res.TimeoutUnit.Name != "download" {
		t.Fatalf("missing timeout unit: %+v", res.TimeoutUnit)
	}
}

func TestPriorityHardOverIdle(t *testing.T) {
	t.Parallel()
	clk := NewManualClock(time.Unix(0, 0))
	started := make(chan struct{})
	resultCh := make(chan Result, 1)

	go func() {
		resultCh <- Run(context.Background(),
			New(Hard(5*time.Second), Idle(5*time.Second), WithClock(clk)),
			func(exec Execution) error {
				close(started)
				<-exec.Context().Done()
				return nil
			},
		)
	}()

	<-started
	clk.Advance(5 * time.Second)
	res := <-resultCh
	if res.Kind != KindHard {
		t.Fatalf("want hard, got %s", res.Kind)
	}
}

func TestUnitEndIdempotent(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), New(), func(exec Execution) error {
		u := exec.BeginUnit("x")
		u.End()
		u.End()
		return nil
	})
	if res.Status != StatusSucceeded {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestConfigReuse(t *testing.T) {
	t.Parallel()
	cfg := New(Hard(time.Hour))
	r1 := cfg.Run(context.Background(), func(Execution) error { return nil })
	r2 := Run(context.Background(), cfg, func(Execution) error { return nil })
	if r1.Status != StatusSucceeded || r2.Status != StatusSucceeded {
		t.Fatalf("r1=%s r2=%s", r1.Status, r2.Status)
	}
}

func TestNegativeDurationInternalError(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), New(Hard(-1)), func(Execution) error { return nil })
	if res.Status != StatusInternalError {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestParentCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	resultCh := make(chan Result, 1)
	go func() {
		resultCh <- Run(ctx, New(), func(exec Execution) error {
			close(started)
			<-exec.Context().Done()
			return nil
		})
	}()
	<-started
	cancel()
	res := <-resultCh
	if res.Status != StatusCanceled {
		t.Fatalf("status=%s", res.Status)
	}
}

func TestConcurrentSignalsRace(t *testing.T) {
	t.Parallel()
	res := Run(context.Background(), New(Hard(time.Hour)), func(exec Execution) error {
		var wg sync.WaitGroup
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				exec.Heartbeat()
				exec.Status("s")
				exec.ProgressTo(int64(i), 100)
				_ = exec.Snapshot()
				u := exec.BeginUnit("u")
				u.End()
			}(i)
		}
		wg.Wait()
		return nil
	})
	if res.Status != StatusSucceeded {
		t.Fatalf("status=%s err=%v", res.Status, res.Err)
	}
}
