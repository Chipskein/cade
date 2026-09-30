// Package procctl finds, stops and detaches cade's own processes, and
// lowers their priority, for ingestions that run in the background
// (issue #41).
package procctl

import "errors"

// ErrUnsupported means this platform cannot do what was asked; only Linux
// is supported for background ingestion.
var ErrUnsupported = errors.New("not supported on this platform")

// DetachedCommand is cade run again, away from the terminal.
type DetachedCommand struct {
	// Args follow the program name.
	Args []string
	// Env is added to this process's environment.
	Env []string
	// LogPath receives stdout and stderr, replaced on every start.
	LogPath string
}

// Processes is what `ingest start`, `stop`, `pause` and `--gentle` need
// from the operating system.
type Processes interface {
	// Alive reports whether pid is a running process.
	Alive(pid int) bool
	// Terminate asks pid to stop (SIGTERM), as Ctrl-C would, waking it
	// first if it is paused.
	Terminate(pid int) error
	// Pause freezes pid (SIGSTOP): no CPU or GPU work, memory kept.
	Pause(pid int) error
	// Continue wakes a paused pid (SIGCONT).
	Continue(pid int) error
	// StartDetached starts this executable in a new session, so closing
	// the terminal does not stop it, and returns its pid.
	StartDetached(command DetachedCommand) (int, error)
	// LowerPriority gives this process the lowest CPU and I/O priority.
	LowerPriority() error
}

// System is Processes on the running operating system.
type System struct{}
