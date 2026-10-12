package rootfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// skipOnDrives skips a test of the names of a system without drives.
func skipOnDrives(t *testing.T) {
	t.Helper()
	if driveOf(Name(".")) != "" {
		t.Skip("names start with the drive here")
	}
}

func TestNameStripsTheRoot(t *testing.T) {
	skipOnDrives(t)
	cases := map[string]string{"/": ".", "/home/me": "home/me", "/home/me/../you/": "home/you"}
	for path, want := range cases {
		if got := Name(path); got != want {
			t.Errorf("Name(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPathInvertsName(t *testing.T) {
	skipOnDrives(t)
	for _, path := range []string{"/", "/home/me/.config"} {
		if got := Path(Name(path)); got != path {
			t.Errorf("Path(Name(%q)) = %q", path, got)
		}
	}
}

func TestNameResolvesRelativePaths(t *testing.T) {
	if got := Name("notes"); got == "notes" || got[0] == '/' {
		t.Fatalf("expected a name under the working directory, got %q", got)
	}
}

func TestSystemOpensANameOfTheWorkingDirectory(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(System(), Name(working)); err != nil {
		t.Fatalf("expected %q readable through System(), got %v", working, err)
	}
}

func TestPathOfANameWritesSlashes(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := Path(Name(working)); got != filepath.ToSlash(working) {
		t.Fatalf("Path(Name(%q)) = %q, expected %q", working, got, filepath.ToSlash(working))
	}
}
