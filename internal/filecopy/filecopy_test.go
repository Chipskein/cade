package filecopy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileCopiesContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("hello"), 0o600)
	err := File(filepath.Join(dir, "a"), filepath.Join(dir, "b"))
	copied, _ := os.ReadFile(filepath.Join(dir, "b"))
	if err != nil || string(copied) != "hello" {
		t.Fatalf("expected hello, got %q (err %v)", copied, err)
	}
}

func TestFileMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := File(filepath.Join(dir, "absent"), filepath.Join(dir, "b")); err == nil {
		t.Fatal("expected an error for a missing source")
	}
}

func TestFlatDirectorySkipsSubdirectories(t *testing.T) {
	source, destination := t.TempDir(), filepath.Join(t.TempDir(), "copy")
	os.WriteFile(filepath.Join(source, "000001.log"), []byte("x"), 0o600)
	os.Mkdir(filepath.Join(source, "nested"), 0o700)
	err := FlatDirectory(source, destination)
	entries, _ := os.ReadDir(destination)
	if err != nil || len(entries) != 1 || entries[0].Name() != "000001.log" {
		t.Fatalf("expected only the regular file, got %v (err %v)", entries, err)
	}
}

func TestFlatDirectoryMissingSource(t *testing.T) {
	if err := FlatDirectory(filepath.Join(t.TempDir(), "absent"), t.TempDir()); err == nil {
		t.Fatal("expected an error for a missing source directory")
	}
}
