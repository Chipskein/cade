// Package dbcopy names the full copies of the database cade writes next to
// it (before a migration that rewrites data, before a rekey) and finds
// them. A copy keeps everything the database held when it was taken, so
// what `forget` deletes later stays in it until the copy goes (#87).
package dbcopy

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/rootfs"
)

// Kinds of copy, as their names say after the database's.
const (
	beforeMigrationPrefix = ".before-v"
	beforeRekeyPrefix     = ".before-rekey-"
	stampLayout           = "20060102-150405"
)

// BeforeMigration names the copy taken before migration version.
//
//	path := dbcopy.BeforeMigration("/data/cade.db", 18, time.Now()) // /data/cade.db.before-v18-20261011-101500
func BeforeMigration(database string, version int, at time.Time) string {
	return fmt.Sprintf("%s%s%d-%s", database, beforeMigrationPrefix, version, at.Format(stampLayout))
}

// BeforeRekey names the copy taken before a rekey.
func BeforeRekey(database string, at time.Time) string {
	return database + beforeRekeyPrefix + at.Format(stampLayout)
}

// Copy is a copy found next to the database.
type Copy struct {
	Path       string
	ModifiedAt time.Time
	Size       int64
}

// Find lists the copies of database in its directory, oldest first, read
// through fsys (the filesystem at "/", see rootfs). The database itself
// and its WAL and shared-memory files are never copies; a directory that
// does not exist has none.
func Find(fsys fs.FS, database string) ([]Copy, error) {
	directory := filepath.Dir(database)
	entries, err := fs.ReadDir(fsys, rootfs.Name(directory))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the copies next to %q: %w", database, err)
	}
	var copies []Copy
	for _, entry := range entries {
		if isCopyOf(filepath.Base(database), entry) {
			copies = appendCopy(copies, filepath.Join(directory, entry.Name()), entry)
		}
	}
	slices.SortFunc(copies, func(a, b Copy) int { return a.ModifiedAt.Compare(b.ModifiedAt) })
	return copies, nil
}

func isCopyOf(database string, entry fs.DirEntry) bool {
	name := entry.Name()
	return entry.Type().IsRegular() && (strings.HasPrefix(name, database+beforeMigrationPrefix) || strings.HasPrefix(name, database+beforeRekeyPrefix))
}

func appendCopy(copies []Copy, path string, entry fs.DirEntry) []Copy {
	info, err := entry.Info()
	if err != nil {
		return copies // removed while listing
	}
	return append(copies, Copy{Path: path, ModifiedAt: info.ModTime(), Size: info.Size()})
}
