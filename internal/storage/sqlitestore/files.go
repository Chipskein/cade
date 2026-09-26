package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// A file is one event holding its current version; file_modifications
// keeps the date and size of every version seen, so the timeline still
// shows each edit without storing (and embedding) the text again.

const createFileModifications = `
CREATE TABLE file_modifications (
	path        TEXT    NOT NULL,
	modified_at INTEGER NOT NULL, -- unix milliseconds, UTC
	size        INTEGER NOT NULL,
	PRIMARY KEY (path, modified_at)
)`

// recordFileModification adds the event's version to the history.
func recordFileModification(ctx context.Context, tx *sql.Tx, ev event.Event) error {
	if ev.Source != event.SourceFile {
		return nil
	}
	file := ev.File()
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO file_modifications (path, modified_at, size) VALUES (?, ?, ?)`,
		file.Path, toUnixMillis(ev.Timestamp), file.Size)
	if err != nil {
		return fmt.Errorf("record modification of %q: %w", file.Path, err)
	}
	return nil
}

// FileModificationsBetween returns the file versions seen in [from, to),
// oldest first.
func (s *Store) FileModificationsBetween(ctx context.Context, from, to time.Time) ([]storage.FileModification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, modified_at, size FROM file_modifications
		WHERE modified_at >= ? AND modified_at < ? ORDER BY modified_at, path`, toUnixMillis(from), toUnixMillis(to))
	if err != nil {
		return nil, fmt.Errorf("query file modifications between %s and %s: %w", from, to, err)
	}
	defer rows.Close()
	var modifications []storage.FileModification
	for rows.Next() {
		var modification storage.FileModification
		var millis int64
		if err := rows.Scan(&modification.Path, &millis, &modification.Size); err != nil {
			return nil, fmt.Errorf("scan file modification: %w", err)
		}
		modification.ModifiedAt = time.UnixMilli(millis)
		modifications = append(modifications, modification)
	}
	return modifications, rows.Err()
}

// MarkMissingFiles sets removed_at on the file events under root that were
// not in present (the paths just collected), and clears it on those that
// came back. Returns how many were newly marked removed.
func (s *Store) MarkMissingFiles(ctx context.Context, root string, present map[string]bool, at time.Time) (int, error) {
	files, err := s.filesUnder(ctx, root)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, file := range files {
		metadata, gone := file.withPresence(present[file.Path], at)
		if metadata == nil {
			continue
		}
		if err := s.storeFileMetadata(ctx, file.eventID, metadata); err != nil {
			return removed, err
		}
		removed += boolToInt(gone)
	}
	return removed, nil
}

type storedFile struct {
	eventID  int64
	metadata event.Metadata
	event.File
}

// withPresence returns the metadata to store when presence changed: a
// vanished file gains removed_at, a returning one loses it.
func (f storedFile) withPresence(present bool, at time.Time) (event.Metadata, bool) {
	removed := !f.RemovedAt.IsZero()
	if present == !removed {
		return nil, false
	}
	file := f.File
	file.RemovedAt = time.Time{}
	if !present {
		file.RemovedAt = at
	}
	return mergeMetadata(f.metadata, file.Metadata()), !present
}

func (s *Store) filesUnder(ctx context.Context, root string) ([]storedFile, error) {
	prefix := strings.TrimSuffix(root, "/") + "/"
	rows, err := s.db.QueryContext(ctx, `SELECT id, metadata FROM events WHERE source = ?
		AND substr(json_extract(metadata, '$.path'), 1, length(?)) = ?`, string(event.SourceFile), prefix, prefix)
	if err != nil {
		return nil, fmt.Errorf("list files under %q: %w", root, err)
	}
	defer rows.Close()
	var files []storedFile
	for rows.Next() {
		var file storedFile
		var raw string
		if err := rows.Scan(&file.eventID, &raw); err != nil {
			return nil, fmt.Errorf("scan file under %q: %w", root, err)
		}
		if file.metadata, err = decodeMetadata(raw); err != nil {
			return nil, err
		}
		file.File = event.Event{Metadata: file.metadata}.File()
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) storeFileMetadata(ctx context.Context, eventID int64, metadata event.Metadata) error {
	encoded, err := encodeMetadata(metadata)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE events SET metadata = ? WHERE id = ?`, encoded, eventID); err != nil {
		return fmt.Errorf("update file event id %d: %w", eventID, err)
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
