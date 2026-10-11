package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
)

// Entities are what several events refer to (a person, a file), derived
// from the events by entity.RefsOf and written with them, like the people
// index. An entity has no dates of its own: they come from its events, so
// forgetting one never leaves a date behind.

var createEntityTables = []string{`
CREATE TABLE IF NOT EXISTS entities (
	id             INTEGER PRIMARY KEY,
	type           TEXT NOT NULL,             -- entity.Type
	entity_key     TEXT NOT NULL,             -- unique within the type: a name key, a path
	canonical_name TEXT NOT NULL,             -- the first spelling stored
	metadata       TEXT NOT NULL DEFAULT '{}', -- JSON object, per type
	UNIQUE (type, entity_key)
)`, `
CREATE TABLE IF NOT EXISTS event_entities (
	event_id  INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
	entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
	role      TEXT    NOT NULL, -- entity.Role
	PRIMARY KEY (event_id, entity_id, role)
)`,
	`CREATE INDEX IF NOT EXISTS event_entities_entity ON event_entities (entity_id)`,
}

func createEntities(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range createEntityTables {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create entity tables: %w", err)
		}
	}
	return nil
}

// linkEntities links the stored event eventID to each entity it refers
// to, creating the entities it is the first to name.
func linkEntities(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	for _, ref := range entity.RefsOf(ev) {
		entityID, err := upsertEntity(ctx, tx, ref)
		if err != nil {
			return fmt.Errorf("store %s %q of event %q: %w", ref.Type, ref.Key, ev.UID, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO event_entities (event_id, entity_id, role) VALUES (?, ?, ?)`, eventID, entityID, string(ref.Role))
		if err != nil {
			return fmt.Errorf("link event %q to %s %q as %s: %w", ev.UID, ref.Type, ref.Key, ref.Role, err)
		}
	}
	return nil
}

// upsertEntity returns the id of the entity ref names; an existing one
// keeps its name. The no-op update is there so RETURNING yields the id
// on a conflict, which DO NOTHING would not.
func upsertEntity(ctx context.Context, tx *sql.Tx, ref entity.Ref) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO entities (type, entity_key, canonical_name) VALUES (?, ?, ?)
		ON CONFLICT (type, entity_key) DO UPDATE SET canonical_name = canonical_name RETURNING id`,
		string(ref.Type), ref.Key, ref.Name).Scan(&id)
	return id, err
}

// relinkEntities replaces the event's links, as an edit replaces its
// people index entries; a name the edit removed goes with its last event.
func relinkEntities(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event) error {
	previous, err := linkedEntityIDs(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_entities WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("unlink entities of event %q: %w", ev.UID, err)
	}
	if err := linkEntities(ctx, tx, eventID, ev); err != nil {
		return err
	}
	return deleteOrphanEntities(ctx, tx, previous)
}

// unlinkEntities removes the event's links and the entities only it
// referred to (PRIVACY, rule 4), before the event is forgotten.
func unlinkEntities(ctx context.Context, tx *sql.Tx, eventID int64) error {
	linked, err := linkedEntityIDs(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_entities WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("unlink entities of event id %d: %w", eventID, err)
	}
	return deleteOrphanEntities(ctx, tx, linked)
}

func linkedEntityIDs(ctx context.Context, tx *sql.Tx, eventID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT entity_id FROM event_entities WHERE event_id = ?`, eventID)
	if err != nil {
		return nil, fmt.Errorf("read entities of event id %d: %w", eventID, err)
	}
	return collectInt64s(rows)
}

// deleteOrphanEntities deletes those of ids that no event links to any
// more; checking only the candidates keeps an edit from scanning every
// entity.
func deleteOrphanEntities(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for _, id := range ids {
		_, err := tx.ExecContext(ctx, `DELETE FROM entities WHERE id = ? AND NOT EXISTS (SELECT 1 FROM event_entities WHERE entity_id = ?)`, id, id)
		if err != nil {
			return fmt.Errorf("delete entity id %d left without events: %w", id, err)
		}
	}
	return nil
}

// deleteEveryOrphanEntity runs after a whole source is forgotten, when
// the candidates are most entities anyway.
func deleteEveryOrphanEntity(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM entities WHERE NOT EXISTS (SELECT 1 FROM event_entities WHERE entity_id = entities.id)`)
	if err != nil {
		return fmt.Errorf("delete entities left without events: %w", err)
	}
	return nil
}
