package testfakes

import (
	"slices"

	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/procctl"
)

// FakeRunStateFile keeps the ingest state in memory and every write made.
type FakeRunStateFile struct {
	State  ingestrun.State
	Found  bool
	Writes []ingestrun.State
}

func (f *FakeRunStateFile) Read() (ingestrun.State, bool, error) {
	return f.State, f.Found, nil
}

func (f *FakeRunStateFile) Write(state ingestrun.State) error {
	f.State, f.Found = state, true
	f.Writes = append(f.Writes, state)
	return nil
}

// FakeIngestLock is held when Held is set, and counts acquisitions and
// releases.
type FakeIngestLock struct {
	Held     bool
	Acquired int
	Released int
}

func (l *FakeIngestLock) Acquire() (func(), error) {
	if l.Held {
		return nil, ingestrun.ErrLocked
	}
	l.Held = true
	l.Acquired++
	return func() {
		l.Held = false
		l.Released++
	}, nil
}

// FakeProcesses answers Alive from AlivePIDs and records what was asked.
type FakeProcesses struct {
	AlivePIDs       []int
	Terminated      []int
	Paused          []int
	Continued       []int
	Started         []procctl.DetachedCommand
	StartedPID      int
	PriorityLowered bool
	PriorityError   error
}

func (p *FakeProcesses) Alive(pid int) bool { return slices.Contains(p.AlivePIDs, pid) }

func (p *FakeProcesses) Terminate(pid int) error {
	p.Terminated = append(p.Terminated, pid)
	return nil
}

func (p *FakeProcesses) Pause(pid int) error {
	p.Paused = append(p.Paused, pid)
	return nil
}

func (p *FakeProcesses) Continue(pid int) error {
	p.Continued = append(p.Continued, pid)
	return nil
}

func (p *FakeProcesses) StartDetached(command procctl.DetachedCommand) (int, error) {
	p.Started = append(p.Started, command)
	return p.StartedPID, nil
}

func (p *FakeProcesses) LowerPriority() error {
	p.PriorityLowered = true
	return p.PriorityError
}
