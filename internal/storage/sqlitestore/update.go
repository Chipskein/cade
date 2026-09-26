package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// StoredEvent returns the stored event with uid, if any.
func (s *Store) StoredEvent(ctx context.Context, uid string) (event.Event, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE uid = ?`, uid)
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
func (s *Store) UpdateEvent(ctx context.Context, ev event.Event, embedding []float32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	eventID, err := updateEventRow(ctx, tx, ev)
	if err != nil {
		return err
	}
	if err := deleteEmbedding(ctx, tx, eventID); err != nil {
		return err
	}
	if err := insertEmbedding(ctx, tx, eventID, ev, embedding); err != nil {
		return err
	}
	return tx.Commit()
}

func updateEventRow(ctx context.Context, tx *sql.Tx, ev event.Event) (int64, error) {
	metadata, err := encodeMetadata(ev.Metadata)
	if err != nil {
		return 0, err
	}
	var eventID int64
	err = tx.QueryRowContext(ctx,
		`UPDATE events SET occurred_at = ?, source = ?, content = ?, metadata = ? WHERE uid = ? RETURNING id`,
		toUnixMillis(ev.Timestamp), string(ev.Source), ev.Content, metadata, ev.UID).Scan(&eventID)
	if err != nil {
		return 0, fmt.Errorf("update event %q, expected it to be stored: %w", ev.UID, err)
	}
	return eventID, nil
}

func deleteEmbedding(ctx context.Context, tx *sql.Tx, eventID int64) error {
	_, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_embeddings WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("delete embedding of event id %d: %w", eventID, err)
	}
	return nil
}
