package sqlitestore

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/storage"
)

var _ storage.EventPager = (*Store)(nil)

// EventsAfter pages by row id, which never changes for a stored event, so
// a walk sees each event once even while ingestion adds others.
func (s *Store) EventsAfter(ctx context.Context, filter storage.EventFilter, after storage.EventCursor, limit int) (storage.EventPage, error) {
	condition := filterCondition(filter)
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+`, events.id FROM events WHERE `+condition.String()+
		` AND events.id > ? ORDER BY events.id LIMIT ?`, append(condition.args, int64(after), limit)...)
	if err != nil {
		return storage.EventPage{}, fmt.Errorf("read events after id %d: %w", after, err)
	}
	defer rows.Close()
	page := storage.EventPage{Next: after}
	for rows.Next() {
		var id int64
		ev, err := scanEvent(rows, &id)
		if err != nil {
			return storage.EventPage{}, err
		}
		page.Events, page.Next = append(page.Events, ev), storage.EventCursor(id)
	}
	page.Done = len(page.Events) < limit
	return page, rows.Err()
}

// RelationsStatedBy lists the relations the event uid states.
func (s *Store) RelationsStatedBy(ctx context.Context, uid string) ([]storage.Relationship, error) {
	rows, err := s.db.QueryContext(ctx, relationshipSelect+`events.uid = ? ORDER BY relationships.id`, "", "", uid)
	if err != nil {
		return nil, fmt.Errorf("read relations stated by %q: %w", uid, err)
	}
	return collectRelationships(rows)
}
