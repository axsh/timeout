package timeout

import (
	"sync"
	"time"
)

// Unit is a named work section within an Execution.
type Unit interface {
	ID() uint64
	Name() string
	End()
}

type unitState struct {
	id        uint64
	name      string
	startedAt time.Time
	ended     bool
}

type unitHandle struct {
	exec *execution
	id   uint64
	name string
}

func (u *unitHandle) ID() uint64   { return u.id }
func (u *unitHandle) Name() string { return u.name }

func (u *unitHandle) End() {
	u.exec.endUnit(u.id)
}

type unitMap struct {
	mu    sync.Mutex
	next  uint64
	units map[uint64]*unitState
}

func newUnitMap() *unitMap {
	return &unitMap{units: make(map[uint64]*unitState)}
}
