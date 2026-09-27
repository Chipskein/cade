package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
)

// The people index (listing.PeopleOf, listing.IndexedDirection) lets a
// person or direction filter run in SQL: without it, a person question
// with no period loaded every event, with content, and filtered in Go.
// It is derived from events.metadata and content, and written with them.

var createPeopleIndex = []string{`
CREATE TABLE event_people (
	event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
	role     TEXT    NOT NULL, -- listing.Role
	name     TEXT    NOT NULL, -- folded words
	name_key TEXT    NOT NULL  -- spelling variants merged (listing.NameKey)
)`,
	`CREATE INDEX event_people_event ON event_people (event_id)`,
	`ALTER TABLE events ADD COLUMN direction TEXT`, // NULL unless a Teams message
}

// indexPeople records who appears in the stored event eventID and, for a
// message, its direction.
func indexPeople(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	for _, entry := range listing.PeopleOf(ev) {
		_, err := tx.ExecContext(ctx, `INSERT INTO event_people (event_id, role, name, name_key) VALUES (?, ?, ?, ?)`,
			eventID, string(entry.Role), entry.Name, entry.Key)
		if err != nil {
			return fmt.Errorf("index %s %q of event %q: %w", entry.Role, entry.Name, ev.UID, err)
		}
	}
	direction := listing.IndexedDirection(ev)
	if direction == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE events SET direction = ? WHERE id = ?`, direction, eventID); err != nil {
		return fmt.Errorf("store direction %q of event %q: %w", direction, ev.UID, err)
	}
	return nil
}

// unindexPeople forgets the event's entries, before an update indexes
// its new version.
func unindexPeople(ctx context.Context, tx *sql.Tx, eventID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_people WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("remove people of event id %d: %w", eventID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE events SET direction = NULL WHERE id = ?`, eventID); err != nil {
		return fmt.Errorf("clear direction of event id %d: %w", eventID, err)
	}
	return nil
}

func reindexPeople(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	if err := unindexPeople(ctx, tx, eventID); err != nil {
		return err
	}
	return indexPeople(ctx, tx, eventID, ev)
}

// peopleIndexPage bounds how many events the migration holds at once.
const peopleIndexPage = 1000

// buildPeopleIndex (schema version 7) creates the index and fills it from
// the stored events, a page at a time. Derived data only: nothing the user
// stored is rewritten.
func buildPeopleIndex(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range createPeopleIndex {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create people index: %w", err)
		}
	}
	for after := int64(0); ; {
		ids, events, err := eventPageAfter(ctx, tx, after)
		if err != nil || len(events) == 0 {
			return err
		}
		for i, ev := range events {
			if err := indexPeople(ctx, tx, ids[i], ev); err != nil {
				return err
			}
		}
		after = ids[len(ids)-1]
	}
}

// eventPageAfter reads the next page of events by id; the page is read
// whole before indexing, as the single connection cannot write while a
// result set is open.
func eventPageAfter(ctx context.Context, tx *sql.Tx, after int64) ([]int64, []event.Event, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+`, id FROM events WHERE id > ? ORDER BY id LIMIT ?`, after, peopleIndexPage)
	if err != nil {
		return nil, nil, fmt.Errorf("read events after id %d: %w", after, err)
	}
	defer rows.Close()
	var ids []int64
	var events []event.Event
	for rows.Next() {
		var id int64
		ev, err := scanEvent(rows, &id)
		if err != nil {
			return nil, nil, err
		}
		ids, events = append(ids, id), append(events, ev)
	}
	return ids, events, rows.Err()
}
