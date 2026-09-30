package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
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

// ftsText is what a chunk is found by: its text and, on the first chunk,
// identifiers the text does not carry (a commit's hash, a file's path).
func ftsText(ev event.Event, chunk storage.Chunk) string {
	text := chunk.Text(ev.Content)
	if chunk.Ordinal != 0 {
		return text
	}
	switch ev.Source {
	case event.SourceGit:
		return text + "\n" + ev.Commit().Hash
	case event.SourceFile:
		return text + "\n" + ev.File().Path
	}
	return text
}

func indexChunk(ctx context.Context, tx *sql.Tx, chunkID int64, ev event.Event, chunk storage.Chunk) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO chunks_fts (rowid, text) VALUES (?, ?)`, chunkID, ftsText(ev, chunk)); err != nil {
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

const lexicalQuery = `
SELECT ` + eventColumns + `, chunks.ordinal, chunks.char_start, chunks.char_end,
	(SELECT COUNT(*) FROM chunks AS siblings WHERE siblings.event_id = chunks.event_id)
FROM chunks_fts
JOIN chunks ON chunks.id = chunks_fts.rowid
JOIN events ON events.id = chunks.event_id
WHERE chunks_fts MATCH ? AND events.occurred_at >= ? AND events.occurred_at < ? AND (? = '' OR events.source = ?)
ORDER BY bm25(chunks_fts)
LIMIT ?`

// SearchLexical returns the chunks matching query.Match (an FTS5
// expression) that satisfy the source and time filters, best BM25 first.
// Hits carry no distance: the caller scores them against the question.
func (s *Store) SearchLexical(ctx context.Context, query storage.LexicalQuery) ([]storage.ScoredEvent, error) {
	if strings.TrimSpace(query.Match) == "" {
		return nil, nil
	}
	from, to := timeBounds(query.From, query.To)
	rows, err := s.db.QueryContext(ctx, lexicalQuery, query.Match, from, to, string(query.Source), string(query.Source), query.Limit)
	if err != nil {
		return nil, fmt.Errorf("keyword search %q: %w", query.Match, err)
	}
	defer rows.Close()
	var hits []storage.ScoredEvent
	for rows.Next() {
		var hit storage.ScoredEvent
		if hit.Event, err = scanEvent(rows, &hit.Chunk.Ordinal, &hit.Chunk.Start, &hit.Chunk.End, &hit.ChunkCount); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
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
