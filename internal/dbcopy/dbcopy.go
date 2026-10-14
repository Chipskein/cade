// Package dbcopy names the full copies of the database cade writes next to
// it (before a migration that rewrites data, a rekey, an encrypt or a
// decrypt) and finds them. A copy keeps everything the database held when
// it was taken, so what `forget` deletes later stays in it until the copy
// goes (#87).
package dbcopy

import (
	"errors"
	"fmt"
	"io/fs"
	pathpkg "path"
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
	beforeEncryptPrefix   = ".before-encrypt-"
	beforeDecryptPrefix   = ".before-decrypt-"
	// convertingSuffix is the database being rewritten by cade encrypt or
	// decrypt; one left by a crash is a full copy like the others.
	convertingSuffix = ".converting"
	stampLayout      = "20060102-150405"
)

// copyMarkers follow the database's name in every kind of copy.
var copyMarkers = []string{beforeMigrationPrefix, beforeRekeyPrefix, beforeEncryptPrefix, beforeDecryptPrefix, convertingSuffix}

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

// BeforeEncrypt names the copy cade encrypt takes, encrypted like the
// result (#111).
func BeforeEncrypt(database string, at time.Time) string {
	return database + beforeEncryptPrefix + at.Format(stampLayout)
}

// BeforeDecrypt names the copy cade decrypt takes, in plain text like the
// result (#111).
func BeforeDecrypt(database string, at time.Time) string {
	return database + beforeDecryptPrefix + at.Format(stampLayout)
}

// Converting names the database while cade encrypt or decrypt rewrites it.
//
//	path := dbcopy.Converting("/data/cade.db") // /data/cade.db.converting
func Converting(database string) string {
	return database + convertingSuffix
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
	database = filepath.ToSlash(database)
	directory := pathpkg.Dir(database)
	entries, err := fs.ReadDir(fsys, rootfs.Name(directory))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list the copies next to %q: %w", database, err)
	}
	var copies []Copy
	for _, entry := range entries {
		if isCopyOf(pathpkg.Base(database), entry) {
			copies = appendCopy(copies, pathpkg.Join(directory, entry.Name()), entry)
		}
	}
	slices.SortFunc(copies, func(a, b Copy) int { return a.ModifiedAt.Compare(b.ModifiedAt) })
	return copies, nil
}

func isCopyOf(database string, entry fs.DirEntry) bool {
	if !entry.Type().IsRegular() {
		return false
	}
	return slices.ContainsFunc(copyMarkers, func(marker string) bool {
		return strings.HasPrefix(entry.Name(), database+marker)
	})
}

func appendCopy(copies []Copy, path string, entry fs.DirEntry) []Copy {
	info, err := entry.Info()
	if err != nil {
		return copies // removed while listing
	}
	return append(copies, Copy{Path: path, ModifiedAt: info.ModTime(), Size: info.Size()})
}
