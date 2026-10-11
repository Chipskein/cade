// Package procctl finds, stops and detaches cade's own processes, and
// lowers their priority, for ingestions that run in the background
// (issue #41).
package procctl

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// logFileMode is owner-only: the log names the ingested targets.
const logFileMode = 0o600

// ErrUnsupported means this platform cannot do what was asked: neither
// unix nor Windows.
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
	// first if it is paused. Windows has no such signal: there it ends
	// pid at once, and `ingest stop` asks for a clean stop first.
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

// startDetached runs this executable with command's arguments, its output
// in command's log, detached from the terminal as attributes say for the
// system, and returns its pid.
func startDetached(command DetachedCommand, attributes *syscall.SysProcAttr) (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locate the cade executable: %w", err)
	}
	logFile, err := os.OpenFile(command.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFileMode)
	if err != nil {
		return 0, fmt.Errorf("open ingest log %q: %w", command.LogPath, err)
	}
	defer logFile.Close()
	process := exec.Command(executable, command.Args...)
	process.Env = append(os.Environ(), command.Env...)
	process.Stdout, process.Stderr = logFile, logFile
	process.SysProcAttr = attributes
	if err := process.Start(); err != nil {
		return 0, fmt.Errorf("start %s %v: %w", executable, command.Args, err)
	}
	// Release sets Pid to -1, so it is read first.
	pid := process.Process.Pid
	return pid, process.Process.Release()
}
