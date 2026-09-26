package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// collapseFileVersions (schema version 4) turns the one-event-per-version
// files of earlier versions into one event per path: every version goes to
// file_modifications, the latest keeps its text and vector under the new
// path-only UID, and the older events and vectors are deleted.
func collapseFileVersions(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, createFileModifications); err != nil {
		return fmt.Errorf("create file_modifications: %w", err)
	}
	versions, err := fileVersions(ctx, tx)
	if err != nil {
		return err
	}
	for path, pathVersions := range versions {
		if err := collapsePath(ctx, tx, path, pathVersions); err != nil {
			return err
		}
	}
	return nil
}

type fileVersion struct {
	eventID    int64
	occurredAt int64
	metadata   event.Metadata
}

// fileVersions groups the file events by path, oldest first.
func fileVersions(ctx context.Context, tx *sql.Tx) (map[string][]fileVersion, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, occurred_at, metadata FROM events WHERE source = ? ORDER BY occurred_at, id`, string(event.SourceFile))
	if err != nil {
		return nil, fmt.Errorf("read file events: %w", err)
	}
	defer rows.Close()
	versions := map[string][]fileVersion{}
	for rows.Next() {
		var version fileVersion
		var raw string
		if err := rows.Scan(&version.eventID, &version.occurredAt, &raw); err != nil {
			return nil, fmt.Errorf("scan file event: %w", err)
		}
		if version.metadata, err = decodeMetadata(raw); err != nil {
			return nil, err
		}
		path := version.metadata["path"]
		versions[path] = append(versions[path], version)
	}
	return versions, rows.Err()
}

func collapsePath(ctx context.Context, tx *sql.Tx, path string, versions []fileVersion) error {
	for _, version := range versions {
		file := event.Event{Metadata: version.metadata}.File()
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO file_modifications (path, modified_at, size) VALUES (?, ?, ?)`,
			path, version.occurredAt, file.Size); err != nil {
			return fmt.Errorf("record version of %q: %w", path, err)
		}
	}
	for _, older := range versions[:len(versions)-1] {
		if err := deleteEventRow(ctx, tx, older.eventID); err != nil {
			return err
		}
	}
	return keepLatestVersion(ctx, tx, path, versions[len(versions)-1])
}

// keepLatestVersion gives the latest version the path-only UID and its
// modification time as revision.
func keepLatestVersion(ctx context.Context, tx *sql.Tx, path string, latest fileVersion) error {
	file := event.Event{Metadata: latest.metadata}.File()
	file.ModifiedAt = time.UnixMilli(latest.occurredAt)
	encoded, err := encodeMetadata(mergeMetadata(latest.metadata, file.Metadata()))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE events SET uid = ?, metadata = ? WHERE id = ?`, event.StableID(event.SourceFile, path), encoded, latest.eventID)
	if err != nil {
		return fmt.Errorf("keep latest version of %q: %w", path, err)
	}
	return nil
}

func deleteEventRow(ctx context.Context, tx *sql.Tx, eventID int64) error {
	if err := deleteEmbedding(ctx, tx, eventID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, eventID); err != nil {
		return fmt.Errorf("delete older version event id %d: %w", eventID, err)
	}
	return nil
}
