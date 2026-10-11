package testfakes

import (
	"io/fs"
	"strings"
	"testing"
)

func TestSystemPathEndsInThePathAndNamesTheFakeFile(t *testing.T) {
	files := NewFakeFileSystem().AddFile("/src/a.md", "a")
	path := SystemPath("/src/a.md")
	if !strings.HasSuffix(path, "/src/a.md") {
		t.Fatalf("expected a path ending in /src/a.md, got %q", path)
	}
	if _, err := fs.Stat(files, strings.TrimPrefix(path, "/")); err != nil {
		t.Fatalf("expected %q to name the fake file, got %v", path, err)
	}
}
