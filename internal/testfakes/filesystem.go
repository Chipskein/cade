package testfakes

import (
	"io/fs"
	"testing/fstest"

	"github.com/chipskein/cade/internal/rootfs"
)

// FakeFileSystem is an in-memory tree rooted at "/", addressed by
// absolute paths; parent directories exist implicitly.
type FakeFileSystem struct {
	fstest.MapFS
}

// NewFakeFileSystem returns an empty tree.
func NewFakeFileSystem() FakeFileSystem {
	return FakeFileSystem{MapFS: fstest.MapFS{}}
}

// AddFile creates the file at the absolute path with content.
func (f FakeFileSystem) AddFile(path, content string) FakeFileSystem {
	f.MapFS[rootfs.Name(path)] = &fstest.MapFile{Data: []byte(content), Mode: 0o600}
	return f
}

// AddDir creates the directory at the absolute path, even if empty.
func (f FakeFileSystem) AddDir(path string) FakeFileSystem {
	f.MapFS[rootfs.Name(path)] = &fstest.MapFile{Mode: fs.ModeDir | 0o700}
	return f
}
