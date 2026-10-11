package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// The source and date filters live inside the vec0 MATCH query, not in an
// outer WHERE: vec0 evaluates metadata constraints during the KNN scan, so
// k counts only chunks that can match (CA9.1). A chunk whose events lie on
// both sides of the period passes the dates with no event inside it, so
// the events decide, and the search widens k until enough chunks pass.
// The source filter uses a sentinel so a single statement covers both
// "any source" and "one source" (vec0 has no OR).
const nearestChunksQuery = `
WITH nearest AS (
	SELECT chunk_id, distance FROM chunk_embeddings
	WHERE embedding MATCH ` + int8VectorValue + ` AND k = ?
	  AND first_at < ? AND last_at >= ?
	  AND source %s ?
)
SELECT chunk_id, distance, EXISTS (SELECT 1 FROM chunks JOIN events ON ` + chunkEvents + `
	WHERE chunks.id = nearest.chunk_id AND events.occurred_at >= ? AND events.occurred_at < ?)
FROM nearest
ORDER BY distance`

// chunkHitsQuery turns the chosen chunks, a JSON array closest first, into
// one hit per chunk and event in the period, newest event first; %s is
// the Among filter.
const chunkHitsQuery = `
SELECT ` + eventColumns + `, chunks.id, chunks.ordinal, chunks.char_start, chunks.char_end, ` + chunkSiblings + `
FROM json_each(?) AS picked
JOIN chunks ON chunks.id = picked.value
JOIN events ON ` + chunkEvents + `
WHERE events.occurred_at >= ? AND events.occurred_at < ? AND %s
ORDER BY picked.key, events.occurred_at DESC, events.id`

// maxNeighbours is sqlite-vec's largest k: a widening that reaches it
// without enough chunks passing has lost some.
const maxNeighbours = 4096

// anySourceSentinel never matches a real source, so "source != sentinel"
// means "all sources".
const anySourceSentinel = "\x00any"

// nearbyChunk is a KNN result: a chunk, its distance and whether one of
// its events is in the period.
type nearbyChunk struct {
	id       int64
	distance float64
	inPeriod bool
}

// SearchSimilar returns the query.Limit chunks nearest to query.Embedding
// that have an event in the source and period, one hit per such event.
func (s *Store) SearchSimilar(ctx context.Context, query storage.SimilarityQuery) ([]storage.ScoredEvent, error) {
	ready, err := s.vectorTableExists(ctx)
	if err != nil || !ready {
		return nil, err
	}
	chunks, err := s.nearestChunks(ctx, query)
	if err != nil || len(chunks) == 0 {
		return nil, err
	}
	return s.chunkHits(ctx, query, chunks)
}

// nearestChunks runs the KNN with k, 4k, 16k... until query.Limit chunks
// have an event in the period, vec0 has no more, or k reaches
// maxNeighbours.
func (s *Store) nearestChunks(ctx context.Context, query storage.SimilarityQuery) ([]nearbyChunk, error) {
	for limit := query.Limit; ; limit = min(limit*4, maxNeighbours) {
		nearby, err := s.nearbyChunks(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		passed := chunksInPeriod(nearby)
		if len(passed) >= query.Limit || len(nearby) < limit || limit >= maxNeighbours {
			return passed[:min(len(passed), query.Limit)], nil
		}
	}
}

func (s *Store) nearbyChunks(ctx context.Context, query storage.SimilarityQuery, limit int) ([]nearbyChunk, error) {
	operator, source := sourceFilter(query.Source)
	from, to := timeBounds(query.From, query.To)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(nearestChunksQuery, operator),
		encodeInt8Vector(query.Embedding), limit, to, from, source, from, to)
	if err != nil {
		return nil, fmt.Errorf("similarity search (k %d, source %q): %w", limit, query.Source, err)
	}
	defer rows.Close()
	var nearby []nearbyChunk
	for rows.Next() {
		var chunk nearbyChunk
		if err := rows.Scan(&chunk.id, &chunk.distance, &chunk.inPeriod); err != nil {
			return nil, fmt.Errorf("scan nearby chunk: %w", err)
		}
		nearby = append(nearby, chunk)
	}
	return nearby, rows.Err()
}

func chunksInPeriod(nearby []nearbyChunk) []nearbyChunk {
	var passed []nearbyChunk
	for _, chunk := range nearby {
		if chunk.inPeriod {
			passed = append(passed, chunk)
		}
	}
	return passed
}

// chunkHits reads the events of chunks in the period and the Among filter.
func (s *Store) chunkHits(ctx context.Context, query storage.SimilarityQuery, chunks []nearbyChunk) ([]storage.ScoredEvent, error) {
	ids, distances := make([]int64, len(chunks)), make(map[int64]float64, len(chunks))
	for i, chunk := range chunks {
		ids[i], distances[chunk.id] = chunk.id, chunk.distance
	}
	picked, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode chunk ids %v: %w", ids, err)
	}
	from, to := timeBounds(query.From, query.To)
	among := amongCondition(query.Among)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(chunkHitsQuery, among), append([]any{string(picked), from, to}, among.args...)...)
	if err != nil {
		return nil, fmt.Errorf("read the events of %d chunks: %w", len(ids), err)
	}
	return collectScoredEvents(rows, distances)
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

func collectScoredEvents(rows *sql.Rows, distances map[int64]float64) ([]storage.ScoredEvent, error) {
	defer rows.Close()
	var hits []storage.ScoredEvent
	for rows.Next() {
		var hit storage.ScoredEvent
		var chunkID int64
		ev, err := scanEvent(rows, &chunkID, &hit.Chunk.Ordinal, &hit.Chunk.Start, &hit.Chunk.End, &hit.ChunkCount)
		if err != nil {
			return nil, err
		}
		hit.Event, hit.Distance = ev, distances[chunkID]
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
