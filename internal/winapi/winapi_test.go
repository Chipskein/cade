//go:build windows

package winapi

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openLockFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func TestLockFileRefusesASecondHandleUntilTheFirstCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ingest.lock")
	first := openLockFile(t, path)
	if err := LockFile(first); err != nil {
		t.Fatalf("expected the first lock, got %v", err)
	}
	if err := LockFile(openLockFile(t, path)); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld for a second handle, got %v", err)
	}
	first.Close()
	if err := LockFile(openLockFile(t, path)); err != nil {
		t.Fatalf("expected the lock free once the first handle closed, got %v", err)
	}
}
