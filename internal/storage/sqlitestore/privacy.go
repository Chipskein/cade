package sqlitestore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// privateFileMode keeps the history readable by its owner only. SQLite
// creates new files with the default mode (0644 under a usual umask), which
// left every message and page readable by other accounts on the machine.
const privateFileMode = 0o600

// sidecarSuffixes are SQLite's companion files in WAL mode; they hold
// recent pages, so they need the same protection as the database.
var sidecarSuffixes = []string{"", "-wal", "-shm"}

// restrictPermissions creates the database file owner-only before SQLite
// opens it (SQLite then gives its companion files the same mode) and
// tightens files left by earlier versions.
func restrictPermissions(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, privateFileMode)
	if err != nil {
		return fmt.Errorf("create database file %q: %w", path, err)
	}
	file.Close()
	for _, suffix := range sidecarSuffixes {
		err := os.Chmod(path+suffix, privateFileMode)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("restrict permissions of %q to %o: %w", path+suffix, privateFileMode, err)
		}
	}
	return nil
}

// compact rewrites the database without the free pages a deletion leaves
// behind and empties the WAL, so `cade forget` removes the text from disk
// instead of only unlinking it.
func (s *Store) compact(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("compact database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("truncate write-ahead log: %w", err)
	}
	return nil
}
