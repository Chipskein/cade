package ingestrun

import "errors"

// ErrLocked means another ingestion holds the lock.
var ErrLocked = errors.New("another cade ingest is running")

// Lock keeps a second ingestion from running at the same time: two would
// compete for the database and the GPU.
type Lock interface {
	// Acquire takes the lock without waiting, or fails with ErrLocked.
	Acquire() (release func(), err error)
}

// FileLock is a lock on a file in Dir.
type FileLock struct {
	Dir Dir
}
