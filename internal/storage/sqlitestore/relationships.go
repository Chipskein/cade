package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
)

// Relationships join two entities through the event that states it
// (entity.RelationsOf): a commit's author changed one of its files. They
// are derived like the entities, and go with their event: deleting it, or
// either entity, deletes the relation (ON DELETE CASCADE).

var createRelationshipTables = []string{`
CREATE TABLE IF NOT EXISTS relationships (
	id              INTEGER PRIMARY KEY,
	from_entity_id  INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
	relation        TEXT    NOT NULL, -- entity.Relation
	to_entity_id    INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
	confidence      REAL,             -- NULL when the source states it; set only when inferred
	source_event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
	metadata        TEXT    NOT NULL DEFAULT '{}', -- JSON object, per relation
	UNIQUE (from_entity_id, relation, to_entity_id, source_event_id)
)`,
	`CREATE INDEX IF NOT EXISTS relationships_from ON relationships (from_entity_id, relation)`,
	`CREATE INDEX IF NOT EXISTS relationships_to ON relationships (to_entity_id, relation)`,
	`CREATE INDEX IF NOT EXISTS relationships_event ON relationships (source_event_id)`,
}

func createRelationships(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range createRelationshipTables {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create relationship tables: %w", err)
		}
	}
	return nil
}

// relateEntities stores the relations the event eventID states; its
// entities are already linked (linkEntities), so both ends exist.
func relateEntities(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	for _, relation := range entity.RelationsOf(ev) {
		_, err := tx.ExecContext(ctx, `INSERT INTO relationships (from_entity_id, relation, to_entity_id, source_event_id)
			VALUES ((SELECT id FROM entities WHERE type = ? AND entity_key = ?), ?, (SELECT id FROM entities WHERE type = ? AND entity_key = ?), ?)`,
			string(relation.From.Type), relation.From.Key, string(relation.Relation), string(relation.To.Type), relation.To.Key, eventID)
		if err != nil {
			return fmt.Errorf("relate %s %q %s %s %q in event %q: %w", relation.From.Type, relation.From.Key, relation.Relation, relation.To.Type, relation.To.Key, ev.UID, err)
		}
	}
	return nil
}

// rerelateEntities replaces the event's relations after an edit relinked
// its entities.
func rerelateEntities(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM relationships WHERE source_event_id = ?`, eventID); err != nil {
		return fmt.Errorf("remove relations of event %q: %w", ev.UID, err)
	}
	return relateEntities(ctx, tx, eventID, ev)
}
