//go:build unix

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
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := process.Start(); err != nil {
		return 0, fmt.Errorf("start %s %v: %w", executable, command.Args, err)
	}
	// Release sets Pid to -1, so it is read first.
	pid := process.Process.Pid
	return pid, process.Process.Release()
}
