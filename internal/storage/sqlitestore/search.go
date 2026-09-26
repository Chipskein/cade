package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// The filters live inside the vec0 MATCH query, not in an outer WHERE: vec0
// evaluates metadata constraints during the KNN scan, so k counts only
// matching rows (CA9.1). The source filter uses a sentinel so a single
// statement covers both "any source" and "one source" (vec0 has no OR).
const similarityQuery = `
WITH nearest AS (
	SELECT event_id, distance FROM event_embeddings
	WHERE embedding MATCH ? AND k = ?
	  AND occurred_at >= ? AND occurred_at < ?
	  AND source %s ?
)
SELECT ` + eventColumns + `, nearest.distance
FROM nearest JOIN events ON events.id = nearest.event_id
ORDER BY nearest.distance`

// anySourceSentinel never matches a real source, so "source != sentinel"
// means "all sources".
const anySourceSentinel = "\x00any"

// SearchSimilar returns the events nearest to query.Embedding that satisfy
// the source and time filters.
func (s *Store) SearchSimilar(ctx context.Context, query storage.SimilarityQuery) ([]storage.ScoredEvent, error) {
	ready, err := s.vectorTableExists(ctx)
	if err != nil || !ready {
		return nil, err
	}
	blob, err := sqlitevec.SerializeFloat32(query.Embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query embedding: %w", err)
	}
	operator, source := sourceFilter(query.Source)
	from, to := timeBounds(query)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(similarityQuery, operator),
		blob, query.Limit, from, to, source)
	if err != nil {
		return nil, fmt.Errorf("similarity search (limit %d, source %q): %w", query.Limit, query.Source, err)
	}
	return collectScoredEvents(rows)
}

func (s *Store) vectorTableExists(ctx context.Context) (bool, error) {
	_, found, err := storedDimensions(ctx, s.db)
	return found, err
}

func sourceFilter(source event.Source) (string, string) {
	if source == "" {
		return "!=", anySourceSentinel
	}
	return "=", string(source)
}

func timeBounds(query storage.SimilarityQuery) (int64, int64) {
	from, to := int64(math.MinInt64), int64(math.MaxInt64)
	if !query.From.IsZero() {
		from = toUnixMillis(query.From)
	}
	if !query.To.IsZero() {
		to = toUnixMillis(query.To)
	}
	return from, to
}

func collectScoredEvents(rows *sql.Rows) ([]storage.ScoredEvent, error) {
	defer rows.Close()
	var hits []storage.ScoredEvent
	for rows.Next() {
		var distance float64
		ev, err := scanEvent(rows, &distance)
		if err != nil {
			return nil, err
		}
		hits = append(hits, storage.ScoredEvent{Event: ev, Distance: distance})
	}
	return hits, rows.Err()
}
