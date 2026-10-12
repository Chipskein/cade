//go:build windows

// Package winapi is cade's thin layer over the Windows calls it needs
// (golang.org/x/sys/windows), so the rest of the code depends on these
// few functions only (#54).
package winapi

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// ErrLockHeld means another handle holds the lock.
var ErrLockHeld = errors.New("the file is locked by another handle")

// lockedBytes is the range LockFile takes: one byte is enough for a lock
// file, and a range may lie past the end of an empty file.
const lockedBytes = 1

// LockFile takes an exclusive lock on file without waiting. Windows drops
// it when the handle closes or the process ends, like flock, so a crashed
// run leaves no stale lock.
//
//	err := winapi.LockFile(file) // winapi.ErrLockHeld if another run has it
func LockFile(file *os.File) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, lockedBytes, 0, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLockHeld
	}
	if err != nil {
		return fmt.Errorf("lock %q: %w", file.Name(), err)
	}
	return nil
}
