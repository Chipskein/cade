// Package privatefile keeps cade's files readable by their owner only.
// On unix that is the file mode; Windows has no such mode (os.Chmod only
// toggles read-only there, and fails on a file SQLite holds open), and a
// file under the user's profile is already private through its ACL (#54).
package privatefile

import "io/fs"

// Restrict gives path mode (0o600 for a file, 0o700 for a directory) where
// the system has file modes, and does nothing on Windows.
//
//	err := privatefile.Restrict("/home/me/.local/share/cade/cade.db", 0o600)
func Restrict(path string, mode fs.FileMode) error {
	return restrict(path, mode)
}
