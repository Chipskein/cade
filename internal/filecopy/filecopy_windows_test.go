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

func TestFileCopiesAnEmptyFileAnotherHandleLocks(t *testing.T) {
	dir := t.TempDir()
	source := lockedByAnotherHandle(t, dir, "LOCK", "")
	if err := File(source, filepath.Join(dir, "copy")); err != nil {
		t.Fatalf("expected the empty LOCK copied, got %v", err)
	}
}
