//go:build windows

package winapi

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports for a process
// that has not ended (STILL_ACTIVE).
const stillActive = 259

// terminatedExitCode is the exit code TerminateProcess gives the process.
const terminatedExitCode = 1

// DetachedCreationFlags start a process without the parent's console and
// in its own group, so closing the terminal or its Ctrl-C does not reach
// it, as setsid does on unix.
const DetachedCreationFlags = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP

// ntdll's NtSuspendProcess and NtResumeProcess freeze and wake every
// thread of a process; Windows has no documented call for SIGSTOP.
var (
	ntdll          = windows.NewLazySystemDLL("ntdll.dll")
	suspendProcess = ntdll.NewProc("NtSuspendProcess")
	resumeProcess  = ntdll.NewProc("NtResumeProcess")
)

// ProcessRunning reports whether pid is a running process; access denied
// means it runs under another user.
//
//	running := winapi.ProcessRunning(1234)
func ProcessRunning(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) == nil && code == stillActive
}

// TerminateProcess ends pid at once, with no chance to clean up.
//
//	err := winapi.TerminateProcess(1234)
func TerminateProcess(pid int) error {
	return withProcess(pid, windows.PROCESS_TERMINATE, func(handle windows.Handle) error {
		return windows.TerminateProcess(handle, terminatedExitCode)
	})
}

// SuspendProcess freezes every thread of pid.
//
//	err := winapi.SuspendProcess(1234)
func SuspendProcess(pid int) error {
	return withProcess(pid, windows.PROCESS_SUSPEND_RESUME, ntCall(suspendProcess))
}

// ResumeProcess wakes a process SuspendProcess froze.
//
//	err := winapi.ResumeProcess(1234)
func ResumeProcess(pid int) error {
	return withProcess(pid, windows.PROCESS_SUSPEND_RESUME, ntCall(resumeProcess))
}

// EnterBackgroundMode gives this process background CPU, I/O and memory
// priority, as nice and the idle I/O class do on Linux. Where background
// mode is refused (wine rejects it), the idle priority class still lowers
// the CPU priority.
//
//	err := winapi.EnterBackgroundMode()
func EnterBackgroundMode() error {
	process := windows.CurrentProcess()
	if err := windows.SetPriorityClass(process, windows.PROCESS_MODE_BACKGROUND_BEGIN); err == nil {
		return nil
	}
	if err := windows.SetPriorityClass(process, windows.IDLE_PRIORITY_CLASS); err != nil {
		return fmt.Errorf("lower the priority to idle: %w", err)
	}
	return nil
}

// withProcess opens pid with access, runs action on it and closes it.
func withProcess(pid int, access uint32, action func(windows.Handle) error) error {
	handle, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(handle)
	return action(handle)
}

// ntCall calls an ntdll function that takes a process handle and returns
// an NTSTATUS, 0 on success.
func ntCall(proc *windows.LazyProc) func(windows.Handle) error {
	return func(handle windows.Handle) error {
		if status, _, _ := proc.Call(uintptr(handle)); status != 0 {
			return fmt.Errorf("%s: NTSTATUS %#x, expected 0", proc.Name, status)
		}
		return nil
	}
}
