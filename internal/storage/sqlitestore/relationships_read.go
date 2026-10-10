package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.RelationIndex = (*Store)(nil)

// relationshipSelect reads a relation with both entities and its event;
// the caller adds the condition on origin or target.
const relationshipSelect = `SELECT origin.id, origin.type, origin.entity_key, origin.canonical_name, relationships.relation,
	target.id, target.type, target.entity_key, target.canonical_name, relationships.confidence, events.uid
	FROM relationships JOIN entities origin ON origin.id = relationships.from_entity_id
	JOIN entities target ON target.id = relationships.to_entity_id JOIN events ON events.id = relationships.source_event_id
	WHERE (? = '' OR relationships.relation = ?) AND `

// RelationsFrom lists the relations whose origin is entityID.
//
//	changed, err := store.RelationsFrom(ctx, rui.ID, entity.RelationChanged)
func (s *Store) RelationsFrom(ctx context.Context, entityID int64, relation entity.Relation) ([]storage.Relationship, error) {
	return s.relationsWhere(ctx, "relationships.from_entity_id = ?", entityID, relation)
}

// RelationsTo lists the relations whose target is entityID.
func (s *Store) RelationsTo(ctx context.Context, entityID int64, relation entity.Relation) ([]storage.Relationship, error) {
	return s.relationsWhere(ctx, "relationships.to_entity_id = ?", entityID, relation)
}

func (s *Store) relationsWhere(ctx context.Context, condition string, entityID int64, relation entity.Relation) ([]storage.Relationship, error) {
	rows, err := s.db.QueryContext(ctx, relationshipSelect+condition+` ORDER BY events.occurred_at, relationships.id`, string(relation), string(relation), entityID)
	if err != nil {
		return nil, fmt.Errorf("read %q relations of entity id %d: %w", relation, entityID, err)
	}
	defer rows.Close()
	var relations []storage.Relationship
	for rows.Next() {
		stored, err := scanRelationship(rows)
		if err != nil {
			return nil, fmt.Errorf("scan relation of entity id %d: %w", entityID, err)
		}
		relations = append(relations, stored)
	}
	return relations, rows.Err()
}

func scanRelationship(rows *sql.Rows) (storage.Relationship, error) {
	var stored storage.Relationship
	var confidence sql.NullFloat64
	err := rows.Scan(&stored.From.ID, &stored.From.Type, &stored.From.Key, &stored.From.Name, &stored.Relation,
		&stored.To.ID, &stored.To.Type, &stored.To.Key, &stored.To.Name, &confidence, &stored.SourceEventUID)
	if confidence.Valid {
		stored.Confidence = &confidence.Float64
	}
	return stored, err
}

// reachableQuery walks relations both ways from the first parameter up to
// the second's hops; UNION drops repeated (entity, hops) rows, and the hop
// bound ends cycles.
const reachableQuery = `WITH RECURSIVE reached(entity_id, hops) AS (
	SELECT ?, 0
	UNION
	SELECT CASE WHEN relationships.from_entity_id = reached.entity_id THEN relationships.to_entity_id ELSE relationships.from_entity_id END, reached.hops + 1
	FROM relationships JOIN reached ON reached.entity_id IN (relationships.from_entity_id, relationships.to_entity_id)
	WHERE reached.hops < ?
)
SELECT entities.id, entities.type, entities.entity_key, entities.canonical_name, MIN(reached.hops) AS fewest
FROM reached JOIN entities ON entities.id = reached.entity_id
WHERE reached.entity_id != ? GROUP BY entities.id ORDER BY fewest, entities.type, entities.entity_key`

// Reachable lists the entities within hops relations of entityID.
//
//	near, err := store.Reachable(ctx, rui.ID, 2) // retry.go (1), Carla (2)
func (s *Store) Reachable(ctx context.Context, entityID int64, hops int) ([]storage.ReachedEntity, error) {
	rows, err := s.db.QueryContext(ctx, reachableQuery, entityID, min(hops, storage.MaxRelationHops), entityID)
	if err != nil {
		return nil, fmt.Errorf("walk %d hops from entity id %d: %w", hops, entityID, err)
	}
	defer rows.Close()
	var reached []storage.ReachedEntity
	for rows.Next() {
		var found storage.ReachedEntity
		if err := rows.Scan(&found.ID, &found.Type, &found.Key, &found.Name, &found.Hops); err != nil {
			return nil, fmt.Errorf("scan entity reached from id %d: %w", entityID, err)
		}
		reached = append(reached, found)
	}
	return reached, rows.Err()
}
