package procctl

import (
	"fmt"
	"syscall"
	"time"

	"github.com/chipskein/cade/internal/winapi"
)

// Alive opens pid and checks it has not exited (#54).
func (System) Alive(pid int) bool {
	return pid > 0 && winapi.ProcessRunning(pid)
}

// stopGrace is how long Terminate waits for pid to stop on its own; a
// variable so tests need not wait that long.
var stopGrace = 30 * time.Second

// exitPollInterval is how often Terminate checks whether pid ended.
const exitPollInterval = 100 * time.Millisecond

// Terminate wakes pid and waits for it to stop on its own: Windows has no
// SIGTERM, so `ingest stop` asks for the stop through the state directory
// first, and the run cancels as on Ctrl-C (#54). A run that has not ended
// after stopGrace is ended at once.
func (System) Terminate(pid int) error {
	if !winapi.ProcessRunning(pid) {
		return fmt.Errorf("stop process %d: not running", pid)
	}
	// A run that is not paused has nothing to resume; if the call fails,
	// the run does not end within stopGrace and TerminateProcess follows.
	_ = winapi.ResumeProcess(pid)
	if exitedWithin(pid, stopGrace) {
		return nil
	}
	return wrapProcessError("stop", pid, winapi.TerminateProcess(pid))
}

// exitedWithin polls until pid has ended or grace has passed.
func exitedWithin(pid int, grace time.Duration) bool {
	for deadline := time.Now().Add(grace); time.Now().Before(deadline); time.Sleep(exitPollInterval) {
		if !winapi.ProcessRunning(pid) {
			return true
		}
	}
	return !winapi.ProcessRunning(pid)
}

func (System) Pause(pid int) error {
	return wrapProcessError("pause", pid, winapi.SuspendProcess(pid))
}

func (System) Continue(pid int) error {
	return wrapProcessError("continue", pid, winapi.ResumeProcess(pid))
}

func wrapProcessError(action string, pid int, err error) error {
	if err != nil {
		return fmt.Errorf("%s process %d: %w", action, pid, err)
	}
	return nil
}

// StartDetached runs without the parent's console and in its own process
// group, so closing the terminal or pressing Ctrl-C there leaves it.
func (System) StartDetached(command DetachedCommand) (int, error) {
	return startDetached(command, &syscall.SysProcAttr{CreationFlags: winapi.DetachedCreationFlags})
}

// LowerPriority enters background mode: low CPU, I/O and memory priority.
func (System) LowerPriority() error {
	if err := winapi.EnterBackgroundMode(); err != nil {
		return fmt.Errorf("lower priority: %w", err)
	}
	return nil
}
