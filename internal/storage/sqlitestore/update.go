package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// StoredEvent returns the stored event with uid, if any.
func (s *Store) StoredEvent(ctx context.Context, uid string) (event.Event, bool, error) {
	return storedEvent(ctx, s.db, uid)
}

func storedEvent(ctx context.Context, querier queryer, uid string) (event.Event, bool, error) {
	row := querier.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE uid = ?`, uid)
	ev, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return event.Event{}, false, nil
	}
	if err != nil {
		return event.Event{}, false, fmt.Errorf("read event %q: %w", uid, err)
	}
	return ev, true, nil
}

// UpdateEvent replaces the row and the embedding of ev.UID atomically, so
// search never sees the new text with the old vector.
func (s *Store) UpdateEvent(ctx context.Context, ev event.Event, chunks []storage.Chunk) error {
	return s.inTransaction(ctx, func(tx *sql.Tx) error {
		return updateEventIn(ctx, tx, ev, chunks)
	})
}

func updateEventIn(ctx context.Context, tx *sql.Tx, ev event.Event, chunks []storage.Chunk) error {
	eventID, err := updateEventRow(ctx, tx, ev)
	if err != nil {
		return err
	}
	if err := reindexPeople(ctx, tx, eventID, ev); err != nil {
		return err
	}
	if err := reindexEventIdentifier(ctx, tx, eventID, ev); err != nil {
		return err
	}
	if err := deleteChunks(ctx, tx, eventID); err != nil {
		return err
	}
	if err := insertChunks(ctx, tx, eventID, ev, chunks); err != nil {
		return err
	}
	return recordFileModification(ctx, tx, ev)
}

// reindexEventIdentifier replaces the event's identifier entry, as an edit
// replaces its chunks' entries.
func reindexEventIdentifier(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	if err := unindexEventIdentifier(ctx, tx, eventID); err != nil {
		return err
	}
	return indexEventIdentifier(ctx, tx, eventID, ev)
}

func updateEventRow(ctx context.Context, tx *sql.Tx, ev event.Event) (int64, error) {
	metadata, err := encodeMetadata(ev.Metadata)
	if err != nil {
		return 0, err
	}
	var eventID int64
	err = tx.QueryRowContext(ctx,
		`UPDATE events SET occurred_at = ?, source = ?, content = ?, metadata = ?, content_hash = ? WHERE uid = ? RETURNING id`,
		toUnixMillis(ev.Timestamp), string(ev.Source), ev.Content, metadata, contentHash(ev.Content), ev.UID).Scan(&eventID)
	if err != nil {
		return 0, fmt.Errorf("update event %q, expected it to be stored: %w", ev.UID, err)
	}
	return eventID, nil
}
