//go:build unix

package procctl

import (
	"errors"
	"fmt"
	"syscall"
)

// Alive sends signal 0, which checks the process without touching it.
// EPERM means it exists under another user.
func (System) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Terminate follows SIGTERM with SIGCONT: a paused process only handles
// the SIGTERM once it runs again.
func (System) Terminate(pid int) error {
	if err := signalProcess(pid, syscall.SIGTERM, "stop"); err != nil {
		return err
	}
	return signalProcess(pid, syscall.SIGCONT, "wake")
}

func (System) Pause(pid int) error { return signalProcess(pid, syscall.SIGSTOP, "pause") }

func (System) Continue(pid int) error { return signalProcess(pid, syscall.SIGCONT, "continue") }

func signalProcess(pid int, signal syscall.Signal, action string) error {
	if err := syscall.Kill(pid, signal); err != nil {
		return fmt.Errorf("%s process %d with %v: %w", action, pid, signal, err)
	}
	return nil
}

// StartDetached runs in a new session (setsid): no controlling terminal,
// so no SIGHUP when it closes. stdin is /dev/null.
func (System) StartDetached(command DetachedCommand) (int, error) {
	return startDetached(command, &syscall.SysProcAttr{Setsid: true})
}
