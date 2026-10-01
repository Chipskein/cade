package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// The filters live inside the vec0 MATCH query, not in an outer WHERE: vec0
// evaluates metadata constraints during the KNN scan, so k counts only
// matching rows (CA9.1). The source filter uses a sentinel so a single
// statement covers both "any source" and "one source" (vec0 has no OR).
const similarityQuery = `
WITH nearest AS (
	SELECT chunk_id, distance FROM chunk_embeddings
	WHERE embedding MATCH ` + int8VectorValue + ` AND k = ?
	  AND occurred_at >= ? AND occurred_at < ?
	  AND source %s ?
)
SELECT ` + eventColumns + `, nearest.distance, chunks.ordinal, chunks.char_start, chunks.char_end,
	(SELECT COUNT(*) FROM chunks AS siblings WHERE siblings.event_id = chunks.event_id)
FROM nearest
JOIN chunks ON chunks.id = nearest.chunk_id
JOIN events ON events.id = chunks.event_id
WHERE %s
ORDER BY nearest.distance`

// anySourceSentinel never matches a real source, so "source != sentinel"
// means "all sources".
const anySourceSentinel = "\x00any"

// SearchSimilar returns the chunks nearest to query.Embedding that satisfy
// the source and time filters, each with its event.
func (s *Store) SearchSimilar(ctx context.Context, query storage.SimilarityQuery) ([]storage.ScoredEvent, error) {
	ready, err := s.vectorTableExists(ctx)
	if err != nil || !ready {
		return nil, err
	}
	blob := encodeInt8Vector(query.Embedding)
	operator, source := sourceFilter(query.Source)
	from, to := timeBounds(query.From, query.To)
	among := amongCondition(query.Among)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(similarityQuery, operator, among),
		append([]any{blob, query.Limit, from, to, source}, among.args...)...)
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

// amongCondition post-filters the nearest chunks' events; none keeps all.
func amongCondition(among *storage.EventFilter) sqlCondition {
	if among == nil {
		return sqlCondition{}
	}
	return filterCondition(*among)
}

// timeBounds turns [start, end) into milliseconds; a zero time is
// unbounded.
func timeBounds(start, end time.Time) (int64, int64) {
	from, to := int64(math.MinInt64), int64(math.MaxInt64)
	if !start.IsZero() {
		from = toUnixMillis(start)
	}
	if !end.IsZero() {
		to = toUnixMillis(end)
	}
	return from, to
}

func collectScoredEvents(rows *sql.Rows) ([]storage.ScoredEvent, error) {
	defer rows.Close()
	var hits []storage.ScoredEvent
	for rows.Next() {
		var hit storage.ScoredEvent
		ev, err := scanEvent(rows, &hit.Distance, &hit.Chunk.Ordinal, &hit.Chunk.Start, &hit.Chunk.End, &hit.ChunkCount)
		if err != nil {
			return nil, err
		}
		hit.Event = ev
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
