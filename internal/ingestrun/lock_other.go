//go:build !unix

package ingestrun

// Acquire does not lock where flock is missing: only Linux is supported,
// and a second run there is refused by the unix build.
func (l FileLock) Acquire() (func(), error) {
	return func() {}, l.Dir.Create()
}
