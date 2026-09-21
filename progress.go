package timeout

import "time"

// Progress is the current execution state for progress reporting.
// It is a present-state event, not a historical log entry.
type Progress struct {
	Stage   string
	Current int64
	Total   int64
	Message string
	Details any
}

type progressSeries struct {
	stage    string
	total    int64
	current  int64
	hasQuant bool
}

// classifyProgress decides Idle/Stall updates for an incoming Progress.
// Same quantitative series is identified by (Stage, Total) in v0.3.
func classifyProgress(prev progressSeries, p Progress) (idleUpdate, stallUpdate bool, next progressSeries) {
	idleUpdate = true

	if p.Total <= 0 {
		// Qualitative progress: updates both Idle and Stall.
		return true, true, progressSeries{stage: p.Stage}
	}

	// Quantitative.
	next = progressSeries{
		stage:    p.Stage,
		total:    p.Total,
		current:  p.Current,
		hasQuant: true,
	}

	if !prev.hasQuant {
		return true, true, next
	}
	if prev.stage != p.Stage || prev.total != p.Total {
		// New series.
		return true, true, next
	}
	if p.Current > prev.current {
		return true, true, next
	}
	// Same or decreased current: activity only.
	return true, false, next
}

func progressSnapshotFrom(p Progress, at time.Time) ProgressSnapshot {
	ps := ProgressSnapshot{
		Stage:     p.Stage,
		Current:   p.Current,
		Total:     p.Total,
		Message:   p.Message,
		Details:   p.Details,
		UpdatedAt: at,
	}
	if p.Total > 0 {
		ps.Ratio = float64(p.Current) / float64(p.Total)
		ps.Percent = ps.Ratio * 100
	}
	return ps
}
