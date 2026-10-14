package sqlitestore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/chipskein/cade/internal/dbcopy"
	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/privatefile"
)

// plainTargetParam makes sqlite3mc write a VACUUM INTO copy in plain text;
// an empty hexkey would encrypt it with the source's key instead.
const plainTargetParam = "key="

// IsEncrypted reports whether the database at path is encrypted: it has
// content and does not start with SQLite's plain header. A missing or
// empty file is not; the first open decides how it is created.
//
//	encrypted, err := sqlitestore.IsEncrypted("/home/me/.local/share/cade/cade.db")
func IsEncrypted(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat database %q: %w", path, err)
	}
	if info.Size() == 0 {
		return false, nil
	}
	plain, err := hasPlainHeader(path)
	if err != nil {
		return false, fmt.Errorf("read the header of database %q: %w", path, err)
	}
	return !plain, nil
}

// copyTargetURI is where VACUUM INTO writes: encrypted with key, or plain
// without one.
func copyTargetURI(path string, key CipherKey) string {
	if len(key) == 0 {
		return "file:" + path + "?" + plainTargetParam
	}
	return "file:" + path + "?" + strings.TrimPrefix(cipherParam, "&") + hexKeyParam + hex.EncodeToString(key)
}

// ReplaceWithKey rewrites the database at path under newKey (empty: plain
// text), reading it with oldKey: `cade encrypt` and `cade decrypt`. It
// first writes backup, a full copy under newKey, then puts a copy of that
// in place of the database; the original changes only at the final
// rename. Nothing else may have the database open.
//
//	err := sqlitestore.ReplaceWithKey(ctx, path, nil, key, dbcopy.BeforeEncrypt(path, time.Now()))
func ReplaceWithKey(ctx context.Context, path string, oldKey, newKey CipherKey, backup string) error {
	if err := copyWithKey(ctx, path, oldKey, backup, newKey); err != nil {
		return err
	}
	partial := dbcopy.Converting(path)
	if err := filecopy.File(backup, partial); err != nil {
		return err
	}
	if err := privatefile.Restrict(partial, privateFileMode); err != nil {
		return fmt.Errorf("restrict %q: %w", partial, err)
	}
	// The old WAL and shared memory would be read as the new file's: the
	// checkpoint emptied the WAL, and the copy already has its pages.
	if err := removeSidecars(path); err != nil {
		return err
	}
	return os.Rename(partial, path)
}

// copyWithKey writes a full copy of the database at source, read with
// sourceKey, to target under targetKey, owner-only.
func copyWithKey(ctx context.Context, source string, sourceKey CipherKey, target string, targetKey CipherKey) error {
	db, err := openKeyed(ctx, source, storeParams, sourceKey)
	if err != nil {
		return err
	}
	defer db.Close()
	// TRUNCATE leaves the WAL empty, so no page of the old file stays
	// behind in it after the rename.
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint %q: %w", source, err)
	}
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, copyTargetURI(target, targetKey)); err != nil {
		return fmt.Errorf("copy %q to %q: %w", source, target, err)
	}
	if err := privatefile.Restrict(target, privateFileMode); err != nil {
		return fmt.Errorf("restrict %q: %w", target, err)
	}
	return db.Close()
}

// removeSidecars deletes the WAL and shared memory of path; the database
// itself (sidecarSuffixes' "") stays until the rename replaces it.
func removeSidecars(path string) error {
	for _, suffix := range sidecarSuffixes {
		if suffix == "" {
			continue
		}
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %q: %w", path+suffix, err)
		}
	}
	return nil
}
