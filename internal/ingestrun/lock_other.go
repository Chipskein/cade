//go:build !unix && !windows

package ingestrun

// Acquire does not lock on a system with neither flock nor LockFileEx;
// cade supports none (the unix and Windows builds lock).
func (l FileLock) Acquire() (func(), error) {
	return func() {}, l.Dir.Create()
}
