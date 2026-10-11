// Package rootfs names the user's files inside an fs.FS rooted at "/", so
// code that only looks at them (`cade init`, `cade doctor`) runs against
// System() in production and an in-memory tree in tests. On Windows the
// first name element is the drive ("C:/Users/me" is the name of
// C:\Users\me), so one filesystem still reaches every drive (#54).
package rootfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// systemRoot is the root of a system without drives.
const systemRoot = "/"

// Name is path's name in a filesystem rooted at "/". A relative path is
// resolved against the working directory first.
//
//	fs.Stat(rootfs.System(), rootfs.Name("/home/me/.config")) // "home/me/.config"
func Name(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = filepath.Clean(systemRoot + path)
	}
	trimmed := strings.Trim(filepath.ToSlash(absolute), "/")
	if trimmed == "" {
		return "."
	}
	return trimmed
}

// Path is the absolute path of name, the inverse of Name. It keeps "/" on
// Windows too, the separator stored paths use (#54).
//
//	rootfs.Path("home/me") // "/home/me"; rootfs.Path("C:/Users/me") // "C:/Users/me" on Windows
func Path(name string) string {
	if name == "." {
		return systemRoot
	}
	if driveOf(name) != "" {
		return name
	}
	return systemRoot + name
}

// System is the user's filesystem under the names Name gives.
//
//	files := rootfs.System()
func System() fs.FS {
	return systemFS{}
}

// systemFS opens a name on its drive, or under "/" where there are none.
type systemFS struct{}

func (systemFS) Open(name string) (fs.File, error) {
	drive := driveOf(name)
	if drive == "" {
		return os.DirFS(systemRoot).Open(name)
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(name, drive), "/")
	if rest == "" {
		rest = "."
	}
	return os.DirFS(drive + systemRoot).Open(rest)
}

// driveOf is name's leading drive ("C:"), always "" outside Windows.
func driveOf(name string) string {
	first, _, _ := strings.Cut(name, "/")
	return filepath.VolumeName(first)
}
