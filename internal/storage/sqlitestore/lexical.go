package sqlitestore

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// chunks_fts indexes chunk text for keyword search. It keeps no copy of the
// text (content=”); contentless_delete lets forget, reindex and edits
// remove a chunk's terms, or deleted text would stay searchable. Accents
// are folded, so "migracao" finds "migração".
const createChunksFTS = `
CREATE VIRTUAL TABLE chunks_fts USING fts5(
	text, content='', contentless_delete=1, tokenize='unicode61 remove_diacritics 2'
)`

// indexChunk indexes the chunk by its text alone: what identifies the
// event goes to event_identifiers_fts.
func indexChunk(ctx context.Context, tx *sql.Tx, chunkID int64, ev event.Event, chunk storage.Chunk) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO chunks_fts (rowid, text) VALUES (?, ?)`, chunkID, chunk.Text(ev.Content)); err != nil {
		return fmt.Errorf("index chunk %d of event %q: %w", chunk.Ordinal, ev.UID, err)
	}
	return nil
}

func unindexChunk(ctx context.Context, tx *sql.Tx, chunkID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE rowid = ?`, chunkID); err != nil {
		return fmt.Errorf("unindex chunk %d: %w", chunkID, err)
	}
	return nil
}

// unindexSource removes the terms of every chunk of source, before its
// events (and, by cascade, chunks) are deleted.
func unindexSource(ctx context.Context, tx *sql.Tx, source event.Source) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE rowid IN
		(SELECT chunks.id FROM chunks JOIN events ON events.id = chunks.event_id WHERE events.source = ?)`, string(source))
	if err != nil {
		return fmt.Errorf("unindex %s chunks: %w", source, err)
	}
	return nil
}

// lexicalQueryTemplate selects a keyword index's hits with their BM25
// score; %[1]s is the index and %[2]s joins its rowid to a chunk and an
// event.
const lexicalQueryTemplate = `
SELECT ` + eventColumns + `, chunks.ordinal, chunks.char_start, chunks.char_end,
	(SELECT COUNT(*) FROM chunks AS siblings WHERE siblings.event_id = chunks.event_id), bm25(%[1]s)
FROM %[1]s %[2]s
WHERE %[1]s MATCH ? AND events.occurred_at >= ? AND events.occurred_at < ? AND (? = '' OR events.source = ?)
ORDER BY bm25(%[1]s)
LIMIT ?`

// lexicalQueries search the chunks' text and the events' identifiers; an
// identifier hit is a hit on the event's first chunk.
var lexicalQueries = []string{
	fmt.Sprintf(lexicalQueryTemplate, "chunks_fts", `JOIN chunks ON chunks.id = chunks_fts.rowid JOIN events ON events.id = chunks.event_id`),
	fmt.Sprintf(lexicalQueryTemplate, "event_identifiers_fts",
		`JOIN events ON events.id = event_identifiers_fts.rowid JOIN chunks ON chunks.event_id = events.id AND chunks.ordinal = 0`),
}

// scoredLexicalHit is a hit with its BM25 score, lower is better.
type scoredLexicalHit struct {
	hit   storage.ScoredEvent
	score float64
}

// SearchLexical returns the chunks matching query.Match (an FTS5
// expression) that satisfy the source and time filters, best BM25 first.
// Hits carry no distance: the caller scores them against the question.
func (s *Store) SearchLexical(ctx context.Context, query storage.LexicalQuery) ([]storage.ScoredEvent, error) {
	if strings.TrimSpace(query.Match) == "" {
		return nil, nil
	}
	var scored []scoredLexicalHit
	for _, statement := range lexicalQueries {
		found, err := s.searchLexicalIndex(ctx, statement, query)
		if err != nil {
			return nil, err
		}
		scored = append(scored, found...)
	}
	return bestLexicalHits(scored, query.Limit), nil
}

func (s *Store) searchLexicalIndex(ctx context.Context, statement string, query storage.LexicalQuery) ([]scoredLexicalHit, error) {
	from, to := timeBounds(query.From, query.To)
	rows, err := s.db.QueryContext(ctx, statement, query.Match, from, to, string(query.Source), string(query.Source), query.Limit)
	if err != nil {
		return nil, fmt.Errorf("keyword search %q: %w", query.Match, err)
	}
	defer rows.Close()
	var scored []scoredLexicalHit
	for rows.Next() {
		var item scoredLexicalHit
		hit := &item.hit
		if hit.Event, err = scanEvent(rows, &hit.Chunk.Ordinal, &hit.Chunk.Start, &hit.Chunk.End, &hit.ChunkCount, &item.score); err != nil {
			return nil, err
		}
		scored = append(scored, item)
	}
	return scored, rows.Err()
}

// bestLexicalHits merges the indexes' hits by score and keeps limit.
func bestLexicalHits(scored []scoredLexicalHit, limit int) []storage.ScoredEvent {
	slices.SortStableFunc(scored, func(a, b scoredLexicalHit) int { return cmp.Compare(a.score, b.score) })
	hits := make([]storage.ScoredEvent, 0, min(len(scored), limit))
	for _, item := range scored[:min(len(scored), limit)] {
		hits = append(hits, item.hit)
	}
	return hits
}

// indexExistingChunks (schema version 6) creates chunks_fts and indexes
// the chunks already stored. Derived data only: no backup.
func indexExistingChunks(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, createChunksFTS); err != nil {
		return fmt.Errorf("create chunks_fts (is the binary built with -tags sqlite_fts5?): %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+`, chunks.id, chunks.ordinal, chunks.char_start, chunks.char_end
		FROM chunks JOIN events ON events.id = chunks.event_id`)
	if err != nil {
		return fmt.Errorf("read chunks to index: %w", err)
	}
	pending, err := chunksToIndex(rows)
	if err != nil {
		return err
	}
	for _, item := range pending {
		if err := indexChunk(ctx, tx, item.id, item.event, item.chunk); err != nil {
			return err
		}
	}
	return nil
}

type chunkToIndex struct {
	id    int64
	chunk storage.Chunk
	event event.Event
}

func chunksToIndex(rows *sql.Rows) ([]chunkToIndex, error) {
	defer rows.Close()
	var pending []chunkToIndex
	for rows.Next() {
		var item chunkToIndex
		var err error
		item.event, err = scanEvent(rows, &item.id, &item.chunk.Ordinal, &item.chunk.Start, &item.chunk.End)
		if err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	return pending, rows.Err()
}

// requireFTS5 fails early, and clearly, when the binary was built without
// the sqlite_fts5 tag (a bare `go build`), instead of an obscure "no such
// module" in the middle of a migration.
func requireFTS5(ctx context.Context, db *sql.DB) error {
	enabled, err := fts5Enabled(ctx, db)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("SQLite was compiled without FTS5; build with `go tool mage build` or `go build -tags sqlite_fts5`")
	}
	return nil
}

// fts5Enabled reports whether this binary's SQLite has FTS5 compiled in.
func fts5Enabled(ctx context.Context, db *sql.DB) (bool, error) {
	var enabled bool
	if err := db.QueryRowContext(ctx, `SELECT sqlite_compileoption_used('ENABLE_FTS5')`).Scan(&enabled); err != nil {
		return false, fmt.Errorf("check SQLite FTS5 support: %w", err)
	}
	return enabled, nil
}
