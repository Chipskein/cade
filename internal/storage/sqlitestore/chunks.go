package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// Chunks belong to a text, not to an event (#67): a page visited 15 times
// is 15 events and one set of chunks, vectors and keyword entries, which
// the events find by their source and content_hash. Every chunk row has
// its vector in chunk_embeddings: they are written and deleted together,
// so an event with text and no chunks is exactly one a reindex still has
// to embed. The chunks go when the last event with their text does.

// createChunksTable is the per-event table of schema versions 5 to 13,
// which migration 5 still creates so version 14 has one layout to convert.
const createChunksTable = `
CREATE TABLE chunks (
	id         INTEGER PRIMARY KEY,
	event_id   INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
	ordinal    INTEGER NOT NULL,
	char_start INTEGER NOT NULL, -- byte offsets into events.content
	char_end   INTEGER NOT NULL,
	UNIQUE (event_id, ordinal)
)`

// createTextChunksTable is the table since version 14. The source is part
// of the text's key so it stays a vec0 filter; no text of the reference
// history is in two sources.
const createTextChunksTable = `
CREATE TABLE text_chunks (
	id           INTEGER PRIMARY KEY,
	source       TEXT    NOT NULL,
	content_hash TEXT    NOT NULL,
	ordinal      INTEGER NOT NULL,
	char_start   INTEGER NOT NULL, -- byte offsets into the events' content
	char_end     INTEGER NOT NULL,
	UNIQUE (source, content_hash, ordinal)
)`

// chunkEvents joins chunks to the events that have their text, and
// chunkSiblings counts the chunks of the same text as chunks.
const (
	chunkEvents   = `events.source = chunks.source AND events.content_hash = chunks.content_hash`
	chunkSiblings = `(SELECT COUNT(*) FROM chunks AS siblings WHERE siblings.source = chunks.source AND siblings.content_hash = chunks.content_hash)`
)

// textKey identifies a text's chunks.
type textKey struct {
	source, contentHash string
}

// storedTextKey is the key of the stored event eventID.
func storedTextKey(ctx context.Context, querier queryer, eventID int64) (textKey, error) {
	var key textKey
	err := querier.QueryRowContext(ctx, `SELECT source, content_hash FROM events WHERE id = ?`, eventID).Scan(&key.source, &key.contentHash)
	if err != nil {
		return key, fmt.Errorf("read the text key of event id %d, expected it to be stored: %w", eventID, err)
	}
	return key, nil
}

// insertChunks gives the stored event eventID its text's chunks: the ones
// already stored, if another event has the same text, or these.
func insertChunks(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event, chunks []storage.Chunk) error {
	key, err := storedTextKey(ctx, tx, eventID)
	if err != nil {
		return err
	}
	ids, err := textChunkIDs(ctx, tx, key)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		if err := insertTextChunks(ctx, tx, key, ev, chunks); err != nil {
			return err
		}
	}
	return refreshTextDates(ctx, tx, key)
}

func insertTextChunks(ctx context.Context, tx *sql.Tx, key textKey, ev event.Event, chunks []storage.Chunk) error {
	for _, chunk := range chunks {
		if err := insertChunk(ctx, tx, key, ev, chunk); err != nil {
			return err
		}
	}
	return nil
}

// insertChunk stores one chunk of ev's text; refreshTextDates then dates
// its vector by all the text's events (a reindex embeds one of many).
func insertChunk(ctx context.Context, tx *sql.Tx, key textKey, ev event.Event, chunk storage.Chunk) error {
	if len(chunk.Vector) == 0 {
		return nil
	}
	if err := ensureVectorTable(ctx, tx, len(chunk.Vector)); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO chunks (source, content_hash, ordinal, char_start, char_end) VALUES (?, ?, ?, ?, ?)`,
		key.source, key.contentHash, chunk.Ordinal, chunk.Start, chunk.End)
	if err != nil {
		return fmt.Errorf("insert chunk %d of event %q: %w", chunk.Ordinal, ev.UID, err)
	}
	chunkID, _ := result.LastInsertId()
	if err := insertVector(ctx, tx, chunkID, ev, chunk.Vector); err != nil {
		return err
	}
	return indexChunk(ctx, tx, chunkID, ev, chunk)
}

// insertVector dates the vector by ev until refreshTextDates runs.
func insertVector(ctx context.Context, tx *sql.Tx, chunkID int64, ev event.Event, vector []float32) error {
	at := toUnixMillis(ev.Timestamp)
	_, err := tx.ExecContext(ctx, `INSERT INTO chunk_embeddings (chunk_id, embedding, source, first_at, last_at) VALUES (?, `+int8VectorValue+`, ?, ?, ?)`,
		chunkID, encodeInt8Vector(vector), string(ev.Source), at, at)
	if err != nil {
		return fmt.Errorf("insert embedding of event %q: %w", ev.UID, err)
	}
	return nil
}

func textChunkIDs(ctx context.Context, querier queryer, key textKey) ([]int64, error) {
	ids, err := queryInts(ctx, querier, `SELECT id FROM chunks WHERE source = ? AND content_hash = ? ORDER BY ordinal`, key.source, key.contentHash)
	if err != nil {
		return nil, fmt.Errorf("list chunks of the %s text %s: %w", key.source, key.contentHash, err)
	}
	return ids, nil
}

// refreshTextDates sets the first and last dates of the text's vectors to
// those of its events, so the KNN's date filter keeps every chunk with an
// event in the period and the dates of a forgotten event leave with it.
func refreshTextDates(ctx context.Context, tx *sql.Tx, key textKey) error {
	ids, err := textChunkIDs(ctx, tx, key)
	if err != nil || len(ids) == 0 {
		return err
	}
	var first, last int64
	err = tx.QueryRowContext(ctx, `SELECT min(occurred_at), max(occurred_at) FROM events WHERE source = ? AND content_hash = ?`,
		key.source, key.contentHash).Scan(&first, &last)
	if err != nil {
		return fmt.Errorf("date the %s text %s, expected an event with it: %w", key.source, key.contentHash, err)
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE chunk_embeddings SET first_at = ?, last_at = ? WHERE chunk_id = ?`, first, last, id); err != nil {
			return fmt.Errorf("date the vector of chunk %d: %w", id, err)
		}
	}
	return nil
}

// releaseText runs after an event leaves the text key, deleted or
// edited: the chunks go with the last event, the others' dates are
// recomputed.
func releaseText(ctx context.Context, tx *sql.Tx, key textKey) error {
	shared, err := eventsWithText(ctx, tx, key)
	if err != nil {
		return err
	}
	if shared > 0 {
		return refreshTextDates(ctx, tx, key)
	}
	return deleteTextChunks(ctx, tx, key)
}

func eventsWithText(ctx context.Context, querier queryer, key textKey) (int, error) {
	var count int
	err := querier.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE source = ? AND content_hash = ?`, key.source, key.contentHash).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events with the %s text %s: %w", key.source, key.contentHash, err)
	}
	return count, nil
}

// deleteTextChunks removes a text's chunks, their vectors (by the vec0
// primary key: a join would scan every vector) and their keyword entries.
func deleteTextChunks(ctx context.Context, tx *sql.Tx, key textKey) error {
	ids, err := textChunkIDs(ctx, tx, key)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE chunk_id = ?`, id); err != nil {
			return fmt.Errorf("delete vector of chunk %d: %w", id, err)
		}
		if err := unindexChunk(ctx, tx, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE source = ? AND content_hash = ?`, key.source, key.contentHash); err != nil {
		return fmt.Errorf("delete chunks of the %s text %s: %w", key.source, key.contentHash, err)
	}
	return nil
}

// replaceEventChunks moves an edited event from its old text to its new
// one. A text no other event has is replaced by these chunks (none leaves
// it without vectors); a shared one keeps its chunks, which are the same.
func replaceEventChunks(ctx context.Context, tx *sql.Tx, eventID int64, old textKey, ev event.Event, chunks []storage.Chunk) error {
	key, err := storedTextKey(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if key != old {
		if err := releaseText(ctx, tx, old); err != nil {
			return err
		}
	}
	shared, err := eventsWithText(ctx, tx, key)
	if err != nil {
		return err
	}
	if shared == 1 {
		if err := deleteTextChunks(ctx, tx, key); err != nil {
			return err
		}
	}
	return insertChunks(ctx, tx, eventID, ev, chunks)
}

// deleteSourceChunks removes the chunks of source; its vectors and
// keyword entries are deleted by source first (delete.go, lexical.go).
func deleteSourceChunks(ctx context.Context, tx *sql.Tx, source event.Source) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE source = ?`, string(source)); err != nil {
		return fmt.Errorf("delete %s chunks: %w", source, err)
	}
	return nil
}

// ChunksFor returns each event's embedded chunks, in order, with vectors.
func (s *Store) ChunksFor(ctx context.Context, uids []string) (map[string][]storage.Chunk, error) {
	chunks := make(map[string][]storage.Chunk, len(uids))
	for _, uid := range uids {
		eventChunks, err := chunksOf(ctx, s.db, uid)
		if err != nil {
			return nil, err
		}
		if len(eventChunks) > 0 {
			chunks[uid] = eventChunks
		}
	}
	return chunks, nil
}

// chunksOf reads the chunk rows first, then each vector by primary key:
// the single connection cannot query while a result set is open.
func chunksOf(ctx context.Context, querier queryer, uid string) ([]storage.Chunk, error) {
	rows, err := querier.QueryContext(ctx, `SELECT chunks.id, ordinal, char_start, char_end FROM chunks
		JOIN events ON `+chunkEvents+` WHERE events.uid = ? ORDER BY ordinal`, uid)
	if err != nil {
		return nil, fmt.Errorf("read chunks of event %q: %w", uid, err)
	}
	ids, chunks, err := scanChunks(rows)
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		if chunks[i].Vector, err = chunkVector(ctx, querier, id); err != nil {
			return nil, err
		}
	}
	return chunks, nil
}

func scanChunks(rows *sql.Rows) ([]int64, []storage.Chunk, error) {
	defer rows.Close()
	var ids []int64
	var chunks []storage.Chunk
	for rows.Next() {
		var id int64
		var chunk storage.Chunk
		if err := rows.Scan(&id, &chunk.Ordinal, &chunk.Start, &chunk.End); err != nil {
			return nil, nil, fmt.Errorf("scan chunk: %w", err)
		}
		ids, chunks = append(ids, id), append(chunks, chunk)
	}
	return ids, chunks, rows.Err()
}

func chunkVector(ctx context.Context, querier queryer, chunkID int64) ([]float32, error) {
	var blob []byte
	if err := querier.QueryRowContext(ctx, `SELECT embedding FROM chunk_embeddings WHERE chunk_id = ?`, chunkID).Scan(&blob); err != nil {
		return nil, fmt.Errorf("read vector of chunk %d: %w", chunkID, err)
	}
	return decodeInt8Vector(blob), nil
}

// queryer is satisfied by both *sql.DB and *sql.Tx, so a read works alone
// or inside an open EventBatch.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func queryInts(ctx context.Context, querier queryer, query string, args ...any) ([]int64, error) {
	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []int64
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
