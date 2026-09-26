package sqlitestore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// eventColumns is the column list every event-returning query selects, in
// the order scanEvent expects.
const eventColumns = `uid, occurred_at, source, content, metadata`

func toUnixMillis(moment time.Time) int64 {
	return moment.UTC().UnixMilli()
}

func fromUnixMillis(millis int64) time.Time {
	return time.UnixMilli(millis).UTC()
}

func encodeMetadata(metadata event.Metadata) (string, error) {
	if metadata == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode metadata %v: %w", metadata, err)
	}
	return string(encoded), nil
}

func decodeMetadata(raw string) (event.Metadata, error) {
	metadata := event.Metadata{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("decode metadata %q, expected a JSON object of strings: %w", raw, err)
	}
	return metadata, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(row rowScanner, extra ...any) (event.Event, error) {
	var ev event.Event
	var millis int64
	var source, rawMetadata string
	targets := append([]any{&ev.UID, &millis, &source, &ev.Content, &rawMetadata}, extra...)
	if err := row.Scan(targets...); err != nil {
		return event.Event{}, fmt.Errorf("scan event row: %w", err)
	}
	metadata, err := decodeMetadata(rawMetadata)
	if err != nil {
		return event.Event{}, err
	}
	ev.Timestamp, ev.Source, ev.Metadata = fromUnixMillis(millis), event.Source(source), metadata
	return ev, nil
}

func collectEvents(rows *sql.Rows) ([]event.Event, error) {
	defer rows.Close()
	var events []event.Event
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}
