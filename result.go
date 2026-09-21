package timeout

import "time"

// ResultStatus is the terminal status of an execution.
type ResultStatus string

const (
	StatusSucceeded     ResultStatus = "succeeded"
	StatusFailed        ResultStatus = "failed"
	StatusTimeout       ResultStatus = "timeout"
	StatusCanceled      ResultStatus = "canceled"
	StatusInternalError ResultStatus = "internal_error"
)

// TimeoutKind identifies which policy fired.
type TimeoutKind string

const (
	KindNone  TimeoutKind = "none"
	KindHard  TimeoutKind = "hard"
	KindIdle  TimeoutKind = "idle"
	KindStall TimeoutKind = "stall"
	KindUnit  TimeoutKind = "unit"
	KindProbe TimeoutKind = "probe"
)

// State is the runtime state of an execution.
type State string

const (
	StateRunning  State = "running"
	StateFinished State = "finished"
)

// ProgressSnapshot is a point-in-time progress view.
type ProgressSnapshot struct {
	Stage     string
	Current   int64
	Total     int64
	Ratio     float64
	Percent   float64
	Message   string
	Details   any
	UpdatedAt time.Time
}

// UnitSnapshot is a point-in-time unit view.
type UnitSnapshot struct {
	ID        uint64
	Name      string
	StartedAt time.Time
	Elapsed   time.Duration
	Limit     time.Duration
	Active    bool
}

// Snapshot is a consistent view of execution state.
type Snapshot struct {
	State          State
	StartedAt      time.Time
	Elapsed        time.Duration
	LastActivityAt time.Time
	IdleFor        time.Duration
	LastProgressAt time.Time
	StallFor       time.Duration
	Status         string
	Progress       ProgressSnapshot
	Units          []UnitSnapshot
}

// Result is the final outcome of Run.
type Result struct {
	Status      ResultStatus
	Kind        TimeoutKind
	Err         error
	Snapshot    Snapshot
	StartedAt   time.Time
	FinishedAt  time.Time
	Elapsed     time.Duration
	TimeoutUnit *UnitSnapshot
}
