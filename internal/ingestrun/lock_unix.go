//go:build unix

package ingestrun

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Acquire takes an flock, which the kernel drops when the process ends, so
// a crashed run never leaves a stale lock behind.
func (l FileLock) Acquire() (func(), error) {
	if err := l.Dir.Create(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(l.Dir.LockPath(), os.O_CREATE|os.O_RDWR, stateFileMode)
	if err != nil {
		return nil, fmt.Errorf("open ingest lock %q: %w", l.Dir.LockPath(), err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %q: %w", l.Dir.LockPath(), err)
	}
	// Closing the file drops the flock.
	return func() { file.Close() }, nil
}
