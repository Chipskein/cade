// Package filecopy copies files and flat directories. Browser databases are
// always read from copies: the live files are locked and must never be
// modified by this program.
package filecopy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// File copies source to destination, creating or truncating destination.
//
//	err := filecopy.File("/profile/History", "/tmp/snap/History")
func File(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %q: %w", source, err)
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create %q: %w", destination, err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return fmt.Errorf("copy %q to %q: %w", source, destination, err)
	}
	return output.Close()
}

// levelDBLockFile is LevelDB's lock: empty, and held by the browser while
// it runs. Windows refuses to read a file another process locks, even an
// empty one, so copying it failed every IndexedDB read with the browser
// open (#54, seen on the Windows CI); no reader needs it.
const levelDBLockFile = "LOCK"

// FlatDirectory copies the regular files directly inside source into
// destination (created if needed); subdirectories and LevelDB's lock
// file are not copied.
func FlatDirectory(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("list %q: %w", source, err)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return fmt.Errorf("create %q: %w", destination, err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || entry.Name() == levelDBLockFile {
			continue
		}
		if err := File(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
