// Package rootfs names the user's files inside an fs.FS rooted at "/", so
// code that only looks at them (`cade init`, `cade doctor`) runs against
// os.DirFS("/") in production and an in-memory tree in tests.
package rootfs

import (
	"path/filepath"
	"strings"
)

// Name is path's name in a filesystem rooted at "/". A relative path is
// resolved against the working directory first.
//
//	fs.Stat(os.DirFS("/"), rootfs.Name("/home/me/.config")) // "home/me/.config"
func Name(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = filepath.Clean("/" + path)
	}
	trimmed := strings.TrimPrefix(absolute, "/")
	if trimmed == "" {
		return "."
	}
	return filepath.ToSlash(trimmed)
}

// Path is the absolute path of name, the inverse of Name.
func Path(name string) string {
	if name == "." {
		return "/"
	}
	return "/" + name
}
