package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
)

// A representation's vector lives in its own vec0 table, int8 and cosine
// like the chunks', created by the first vector saved: no model writes one
// yet (a visual embedding is future work), and its dimension is only known
// then. vec0 has no foreign keys, so triggers drop a vector with its
// representation, cascades included, and when its content is replaced.

const representationDimensionsKey = "representation_vector_dimensions"

const createRepresentationVectorTemplate = `
CREATE VIRTUAL TABLE IF NOT EXISTS representation_vectors USING vec0(
	representation_id INTEGER PRIMARY KEY,
	embedding         int8[%d] distance_metric=cosine,
	kind              TEXT
)`

var representationVectorTriggers = []string{`
CREATE TRIGGER IF NOT EXISTS representation_vectors_deleted AFTER DELETE ON representations BEGIN
	DELETE FROM representation_vectors WHERE representation_id = old.id;
END`, `
CREATE TRIGGER IF NOT EXISTS representation_vectors_replaced AFTER UPDATE OF content ON representations
WHEN old.content IS NOT new.content BEGIN
	DELETE FROM representation_vectors WHERE representation_id = old.id;
END`,
}

// SaveRepresentationVector stores vector for the representation.
//
//	err := store.SaveRepresentationVector(ctx, id, embedding)
func (s *Store) SaveRepresentationVector(ctx context.Context, representationID int64, vector []float32) error {
	return s.inTransaction(ctx, func(tx *sql.Tx) error {
		kind, err := representationKind(ctx, tx, representationID)
		if err != nil {
			return err
		}
		if err := ensureRepresentationVectors(ctx, tx, len(vector)); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO representation_vectors (representation_id, embedding, kind) VALUES (?, `+int8VectorValue+`, ?)`,
			representationID, encodeInt8Vector(vector), kind)
		if err != nil {
			return fmt.Errorf("store the vector of representation id %d: %w", representationID, err)
		}
		return nil
	})
}

func representationKind(ctx context.Context, tx *sql.Tx, representationID int64) (string, error) {
	var kind string
	err := tx.QueryRowContext(ctx, `SELECT kind FROM representations WHERE id = ?`, representationID).Scan(&kind)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("representation id %d is not stored, expected an id from SaveRepresentation", representationID)
	}
	return kind, err
}

// ensureRepresentationVectors creates the table at the first vector's
// dimension and refuses any other later.
func ensureRepresentationVectors(ctx context.Context, tx *sql.Tx, dimensions int) error {
	stored, found, err := representationDimensions(ctx, tx)
	if err != nil || found && stored == dimensions {
		return err
	}
	if found {
		return fmt.Errorf("representation vector has %d dimensions, the index holds %d", dimensions, stored)
	}
	statements := append([]string{fmt.Sprintf(createRepresentationVectorTemplate, dimensions)}, representationVectorTriggers...)
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create representation vectors with %d dimensions: %w", dimensions, err)
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO store_settings (key, value) VALUES (?, ?)`, representationDimensionsKey, strconv.Itoa(dimensions))
	return err
}

func representationDimensions(ctx context.Context, querier queryRower) (int, bool, error) {
	var raw string
	err := querier.QueryRowContext(ctx, `SELECT value FROM store_settings WHERE key = ?`, representationDimensionsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read representation vector dimensions: %w", err)
	}
	dimensions, err := strconv.Atoi(raw)
	return dimensions, true, err
}

// nearestRepresentationsQuery filters the kind inside the KNN scan, as the
// chunk search does with the source.
const nearestRepresentationsQuery = `
WITH nearest AS (
	SELECT representation_id, distance FROM representation_vectors
	WHERE embedding MATCH ` + int8VectorValue + ` AND k = ? AND kind = ?
)
SELECT representations.id, modality, kind, model, revision, content, entities.type, entities.entity_key, nearest.distance
FROM nearest JOIN representations ON representations.id = nearest.representation_id
JOIN entities ON entities.id = representations.entity_id ORDER BY nearest.distance`

// NearestRepresentations lists the representations of kind closest to
// vector; none before any vector is stored.
func (s *Store) NearestRepresentations(ctx context.Context, vector []float32, kind representation.Kind, limit int) ([]storage.ScoredRepresentation, error) {
	if _, found, err := representationDimensions(ctx, s.db); err != nil || !found {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, nearestRepresentationsQuery, encodeInt8Vector(vector), limit, string(kind))
	if err != nil {
		return nil, fmt.Errorf("search the %d nearest %s representations: %w", limit, kind, err)
	}
	defer rows.Close()
	var scored []storage.ScoredRepresentation
	for rows.Next() {
		var hit storage.ScoredRepresentation
		if err := rows.Scan(&hit.ID, &hit.Modality, &hit.Kind, &hit.Model, &hit.Revision, &hit.Content, &hit.Owner.Type, &hit.Owner.Key, &hit.Distance); err != nil {
			return nil, fmt.Errorf("scan a %s representation found by vector: %w", kind, err)
		}
		scored = append(scored, hit)
	}
	return scored, rows.Err()
}
