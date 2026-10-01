package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
)

// shareChunksByText (schema version 14) keeps one set of chunks, vectors
// and keyword entries per text (#67): of the events with the same source
// and content_hash, the first one with chunks keeps them, under the
// text's key, and the copies of the others are deleted. The vector table
// is then rewritten, which dates each vector by all its text's events and
// leaves no empty position. It rewrites data: the step backs up.
func shareChunksByText(ctx context.Context, tx *sql.Tx) error {
	if keyed, err := chunksKeyedByEvent(ctx, tx); err != nil || !keyed {
		return err
	}
	if err := execMigration(ctx, tx, createTextChunksTable, keepFirstChunksOfEachText); err != nil {
		return err
	}
	if err := deleteChunkCopies(ctx, tx); err != nil {
		return err
	}
	if err := execMigration(ctx, tx, `DROP TABLE chunks`, `ALTER TABLE text_chunks RENAME TO chunks`); err != nil {
		return err
	}
	dimensions, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	return rewriteVectorTable(ctx, tx, dimensions, textDatesRewrite)
}

// keepFirstChunksOfEachText copies, with their ids, the chunks of the
// first event with chunks of each text: its vectors and keyword entries
// stay valid as they are.
const keepFirstChunksOfEachText = `
INSERT INTO text_chunks (id, source, content_hash, ordinal, char_start, char_end)
SELECT chunks.id, events.source, events.content_hash, chunks.ordinal, chunks.char_start, chunks.char_end
FROM chunks JOIN events ON events.id = chunks.event_id
WHERE chunks.event_id IN (
	SELECT min(chunks.event_id) FROM chunks JOIN events ON events.id = chunks.event_id
	GROUP BY events.source, events.content_hash)`

// textDatesRewrite keeps each vector as stored and dates it by the first
// and last events with its text.
var textDatesRewrite = vectorRewrite{componentBytes: int8Bytes, convert: keepVectorBlob, layout: dateRangeLayout,
	readMetadata: `source,
		(SELECT min(events.occurred_at) FROM chunks JOIN events ON ` + chunkEvents + ` WHERE chunks.id = chunk_embeddings.chunk_id),
		(SELECT max(events.occurred_at) FROM chunks JOIN events ON ` + chunkEvents + ` WHERE chunks.id = chunk_embeddings.chunk_id)`}

// deleteChunkCopies deletes the vectors (by the vec0 primary key) and
// keyword entries of the chunks not kept.
func deleteChunkCopies(ctx context.Context, tx *sql.Tx) error {
	copies, err := queryInts(ctx, tx, `SELECT id FROM chunks WHERE id NOT IN (SELECT id FROM text_chunks)`)
	if err != nil {
		return fmt.Errorf("list the chunk copies: %w", err)
	}
	for _, id := range copies {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE chunk_id = ?`, id); err != nil {
			return fmt.Errorf("delete the vector of chunk copy %d: %w", id, err)
		}
		if err := unindexChunk(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// chunksKeyedByEvent is false for a table already keyed by text (created
// by this version, then marked older, as other migration tests do).
func chunksKeyedByEvent(ctx context.Context, tx *sql.Tx) (bool, error) {
	var keyed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pragma_table_info('chunks') WHERE name = 'event_id')`).Scan(&keyed)
	if err != nil {
		return false, fmt.Errorf("read the columns of chunks: %w", err)
	}
	return keyed, nil
}

func execMigration(ctx context.Context, tx *sql.Tx, statements ...string) error {
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("share chunks by text, at %q: %w", statement, err)
		}
	}
	return nil
}
