package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// keepTeamsText (schema version 2) stores each Teams message's own text in
// its metadata, recovered from the stored content. With it, a change to how
// content is laid out can rebuild content from the database instead of
// re-ingesting from a cache that has expired. A message whose content does
// not round-trip exactly (an older layout) is left without text.
func keepTeamsText(ctx context.Context, tx *sql.Tx) error {
	updates, err := teamsTextUpdates(ctx, tx)
	if err != nil {
		return err
	}
	for id, metadata := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET metadata = ? WHERE id = ?`, metadata, id); err != nil {
			return fmt.Errorf("store text of event id %d: %w", id, err)
		}
	}
	return nil
}

// teamsTextUpdates reads every Teams event first: the single connection
// cannot write while a result set is open.
func teamsTextUpdates(ctx context.Context, tx *sql.Tx) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, content, metadata FROM events WHERE source = ?`, string(event.SourceTeams))
	if err != nil {
		return nil, fmt.Errorf("read Teams events: %w", err)
	}
	defer rows.Close()
	updates := map[int64]string{}
	for rows.Next() {
		id, metadata, changed, err := withRecoveredText(rows)
		if err != nil {
			return nil, err
		}
		if changed {
			updates[id] = metadata
		}
	}
	return updates, rows.Err()
}

func withRecoveredText(rows *sql.Rows) (int64, string, bool, error) {
	var id int64
	var content, raw string
	if err := rows.Scan(&id, &content, &raw); err != nil {
		return 0, "", false, fmt.Errorf("scan Teams event: %w", err)
	}
	metadata, err := decodeMetadata(raw)
	if err != nil {
		return 0, "", false, err
	}
	message := event.Event{Metadata: metadata}.Message()
	text, ok := event.MessageText(content, message)
	if message.Text != "" || !ok {
		return id, "", false, nil
	}
	message.Text = text
	encoded, err := encodeMetadata(mergeMetadata(metadata, message.Metadata()))
	return id, encoded, err == nil, err
}

// mergeMetadata keeps keys the typed view does not know about.
func mergeMetadata(stored, typed event.Metadata) event.Metadata {
	merged := event.Metadata{}
	for key, value := range stored {
		merged[key] = value
	}
	for key, value := range typed {
		merged[key] = value
	}
	return merged
}
