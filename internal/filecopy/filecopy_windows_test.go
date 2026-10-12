package filecopy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/winapi"
)

// lockedByAnotherHandle writes content to name and locks it from another
// handle, as a running browser holds its LevelDB LOCK file (#54).
func lockedByAnotherHandle(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	holder, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { holder.Close() })
	if err := winapi.LockFile(holder); err != nil {
		t.Fatal(err)
	}
	return path
}

// Regression (#54): with the browser open, its LevelDB LOCK is locked, and
// Windows refused to copy it, failing the whole IndexedDB read.
func TestFlatDirectoryCopiesALevelDBTheBrowserHolds(t *testing.T) {
	source := t.TempDir()
	lockedByAnotherHandle(t, source, levelDBLockFile, "")
	if err := os.WriteFile(filepath.Join(source, "CURRENT"), []byte("MANIFEST-000001"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := FlatDirectory(source, filepath.Join(t.TempDir(), "copy")); err != nil {
		t.Fatalf("expected the LevelDB copied without its lock, got %v", err)
	}
}
