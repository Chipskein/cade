package procctl

import (
	"fmt"
	"syscall"

	"github.com/chipskein/cade/internal/winapi"
)

// Alive opens pid and checks it has not exited (#54).
func (System) Alive(pid int) bool {
	return pid > 0 && winapi.ProcessRunning(pid)
}

// Terminate ends pid at once: Windows has no SIGTERM to deliver, so a
// clean stop is asked for through the state directory first (ingestrun).
// A suspended process ends too.
func (System) Terminate(pid int) error {
	return wrapProcessError("stop", pid, winapi.TerminateProcess(pid))
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
