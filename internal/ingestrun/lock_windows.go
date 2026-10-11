package ingestrun

import (
	"errors"
	"fmt"
	"os"

	"github.com/chipskein/cade/internal/winapi"
)

// Acquire takes a LockFileEx lock, which Windows drops when the process
// ends, as the kernel drops an flock on unix (#54).
func (l FileLock) Acquire() (func(), error) {
	if err := l.Dir.Create(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(l.Dir.LockPath(), os.O_CREATE|os.O_RDWR, stateFileMode)
	if err != nil {
		return nil, fmt.Errorf("open ingest lock %q: %w", l.Dir.LockPath(), err)
	}
	if err := winapi.LockFile(file); err != nil {
		file.Close()
		if errors.Is(err, winapi.ErrLockHeld) {
			return nil, ErrLocked
		}
		return nil, err
	}
	// Closing the handle drops the lock.
	return func() { file.Close() }, nil
}
