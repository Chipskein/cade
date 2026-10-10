package sqlitestore

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/storage"
)

var _ storage.EntityIndex = (*Store)(nil)

// EntitiesOfEvent lists what the event uid refers to.
//
//	links, err := store.EntitiesOfEvent(ctx, "teams:19:abc/1758820")
func (s *Store) EntitiesOfEvent(ctx context.Context, uid string) ([]storage.EntityLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT entities.id, entities.type, entities.entity_key, entities.canonical_name, event_entities.role
		FROM event_entities JOIN entities ON entities.id = event_entities.entity_id JOIN events ON events.id = event_entities.event_id
		WHERE events.uid = ? ORDER BY entities.type, entities.entity_key, event_entities.role`, uid)
	if err != nil {
		return nil, fmt.Errorf("read entities of event %q: %w", uid, err)
	}
	defer rows.Close()
	var links []storage.EntityLink
	for rows.Next() {
		var link storage.EntityLink
		if err := rows.Scan(&link.ID, &link.Type, &link.Key, &link.Name, &link.Role); err != nil {
			return nil, fmt.Errorf("scan entity of event %q: %w", uid, err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// EventsOfEntity lists the UIDs of the events referring to entityID.
//
//	uids, err := store.EventsOfEntity(ctx, links[0].ID)
func (s *Store) EventsOfEntity(ctx context.Context, entityID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uid FROM events
		WHERE id IN (SELECT event_id FROM event_entities WHERE entity_id = ?) ORDER BY occurred_at, id`, entityID)
	if err != nil {
		return nil, fmt.Errorf("read events of entity id %d: %w", entityID, err)
	}
	return collectStrings(rows)
}
