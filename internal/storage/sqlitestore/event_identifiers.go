package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// A commit's hash and a file's path are found by keyword, but they belong
// to the event, not to its text: two commits "Merge branch 'dev'" share a
// text and not a hash, and since #67 they share its chunks too. They live
// in their own contentless index, keyed by event id, and a keyword hit on
// one is a hit on the event's first chunk.
const createEventIdentifiersFTS = `
CREATE VIRTUAL TABLE IF NOT EXISTS event_identifiers_fts USING fts5(
	identifier, content='', contentless_delete=1, tokenize='unicode61 remove_diacritics 2'
)`

// eventIdentifier is what an event is found by besides its text; "" for
// sources whose text says it all.
//
//	eventIdentifier(commit) // "e5f6a7b8c9..."
func eventIdentifier(ev event.Event) string {
	switch ev.Source {
	case event.SourceGit:
		return ev.Commit().Hash
	case event.SourceFile:
		return ev.File().Path
	}
	return ""
}

func indexEventIdentifier(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	identifier := eventIdentifier(ev)
	if identifier == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO event_identifiers_fts (rowid, identifier) VALUES (?, ?)`, eventID, identifier); err != nil {
		return fmt.Errorf("index identifier of event %q: %w", ev.UID, err)
	}
	return nil
}

func unindexEventIdentifier(ctx context.Context, tx *sql.Tx, eventID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_identifiers_fts WHERE rowid = ?`, eventID); err != nil {
		return fmt.Errorf("unindex identifier of event id %d: %w", eventID, err)
	}
	return nil
}

// unindexSourceIdentifiers runs before the source's events are deleted.
func unindexSourceIdentifiers(ctx context.Context, tx *sql.Tx, source event.Source) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM event_identifiers_fts WHERE rowid IN (SELECT id FROM events WHERE source = ?)`, string(source))
	if err != nil {
		return fmt.Errorf("unindex %s identifiers: %w", source, err)
	}
	return nil
}

// identifiedEvent is an event with an identifier and, if it has one, its
// first chunk, whose index entry still carries the identifier.
type identifiedEvent struct {
	id      int64
	event   event.Event
	chunkID sql.NullInt64
	chunk   storage.Chunk
}

// moveIdentifiersToEvents (schema version 12) indexes each commit's hash
// and file's path by event, and indexes their first chunk again without
// it. Derived data only: no backup. Each entry is replaced, not added, so
// running it again changes nothing.
func moveIdentifiersToEvents(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, createEventIdentifiersFTS); err != nil {
		return fmt.Errorf("create event_identifiers_fts: %w", err)
	}
	identified, err := identifiedEvents(ctx, tx)
	if err != nil {
		return err
	}
	for _, item := range identified {
		if err := moveIdentifier(ctx, tx, item); err != nil {
			return err
		}
	}
	return nil
}

// identifiedEvents reads them all first: the single connection cannot
// write while a result set is open.
func identifiedEvents(ctx context.Context, tx *sql.Tx) ([]identifiedEvent, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+`, events.id, chunks.id, COALESCE(chunks.char_start, 0), COALESCE(chunks.char_end, 0)
		FROM events LEFT JOIN chunks ON chunks.event_id = events.id AND chunks.ordinal = 0
		WHERE events.source IN (?, ?)`, string(event.SourceGit), string(event.SourceFile))
	if err != nil {
		return nil, fmt.Errorf("read commits and files to index by identifier: %w", err)
	}
	defer rows.Close()
	var identified []identifiedEvent
	for rows.Next() {
		var item identifiedEvent
		if item.event, err = scanEvent(rows, &item.id, &item.chunkID, &item.chunk.Start, &item.chunk.End); err != nil {
			return nil, err
		}
		identified = append(identified, item)
	}
	return identified, rows.Err()
}

func moveIdentifier(ctx context.Context, tx *sql.Tx, item identifiedEvent) error {
	if err := reindexEventIdentifier(ctx, tx, item.id, item.event); err != nil {
		return err
	}
	if !item.chunkID.Valid {
		return nil
	}
	if err := unindexChunk(ctx, tx, item.chunkID.Int64); err != nil {
		return err
	}
	return indexChunk(ctx, tx, item.chunkID.Int64, item.event, item.chunk)
}
