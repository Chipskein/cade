package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.RepresentationIndex = (*Store)(nil)

// Representations belong to an entity, not to an event: copies of one
// image share its description. They go with the entity (ON DELETE
// CASCADE), which goes with its last event.

var createRepresentationTables = []string{`
CREATE TABLE IF NOT EXISTS representations (
	id        INTEGER PRIMARY KEY,
	entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
	modality  TEXT    NOT NULL, -- representation.Modality
	kind      TEXT    NOT NULL, -- representation.Kind
	model     TEXT    NOT NULL,
	revision  INTEGER NOT NULL DEFAULT 0, -- e.g. the caption prompt version
	content   TEXT    NOT NULL,
	metadata  TEXT    NOT NULL DEFAULT '{}', -- JSON object, per kind
	UNIQUE (entity_id, modality, kind)
)`,
	`CREATE INDEX IF NOT EXISTS representations_model ON representations (kind, model, revision)`,
}

// createRepresentations is schema version 17.
func createRepresentations(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range createRepresentationTables {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create representation tables: %w", err)
		}
	}
	return nil
}

// SaveRepresentation stores rep for owner, replacing the one of the same
// modality and kind in place.
//
//	id, err := store.SaveRepresentation(ctx, storage.EntityKey{Type: entity.TypeImage, Key: sha}, rep)
func (s *Store) SaveRepresentation(ctx context.Context, owner storage.EntityKey, rep representation.Representation) (int64, error) {
	var id int64
	err := s.inTransaction(ctx, func(tx *sql.Tx) (err error) {
		id, err = saveRepresentation(ctx, tx, owner, rep)
		return err
	})
	return id, err
}

func saveRepresentation(ctx context.Context, querier queryRower, owner storage.EntityKey, rep representation.Representation) (int64, error) {
	var id int64
	err := querier.QueryRowContext(ctx, `INSERT INTO representations (entity_id, modality, kind, model, revision, content)
		SELECT id, ?, ?, ?, ?, ? FROM entities WHERE type = ? AND entity_key = ?
		ON CONFLICT (entity_id, modality, kind) DO UPDATE SET model = excluded.model, revision = excluded.revision, content = excluded.content
		RETURNING id`, string(rep.Modality), string(rep.Kind), rep.Model, rep.Revision, rep.Content, string(owner.Type), owner.Key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store %s %s of %s %q: no such entity, expected one an event refers to", rep.Modality, rep.Kind, owner.Type, owner.Key)
	}
	if err != nil {
		return 0, fmt.Errorf("store %s %s of %s %q: %w", rep.Modality, rep.Kind, owner.Type, owner.Key, err)
	}
	return id, nil
}

// RepresentationsOf lists owner's representations by modality and kind.
func (s *Store) RepresentationsOf(ctx context.Context, owner storage.EntityKey) ([]storage.StoredRepresentation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT representations.id, modality, kind, model, revision, content
		FROM representations JOIN entities ON entities.id = representations.entity_id
		WHERE entities.type = ? AND entities.entity_key = ? ORDER BY modality, kind`, string(owner.Type), owner.Key)
	if err != nil {
		return nil, fmt.Errorf("read representations of %s %q: %w", owner.Type, owner.Key, err)
	}
	defer rows.Close()
	var stored []storage.StoredRepresentation
	for rows.Next() {
		var rep storage.StoredRepresentation
		if err := rows.Scan(&rep.ID, &rep.Modality, &rep.Kind, &rep.Model, &rep.Revision, &rep.Content); err != nil {
			return nil, fmt.Errorf("scan representation of %s %q: %w", owner.Type, owner.Key, err)
		}
		stored = append(stored, rep)
	}
	return stored, rows.Err()
}
