package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// Every chunk row has its vector in chunk_embeddings: they are written and
// deleted together, so an event with text and no chunks is exactly one a
// reindex still has to embed.

const createChunksTable = `
CREATE TABLE chunks (
	id         INTEGER PRIMARY KEY,
	event_id   INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
	ordinal    INTEGER NOT NULL,
	char_start INTEGER NOT NULL, -- byte offsets into events.content
	char_end   INTEGER NOT NULL,
	UNIQUE (event_id, ordinal)
)`

func insertChunks(ctx context.Context, tx *sql.Tx, eventID int64, ev event.Event, chunks []storage.Chunk) error {
	for _, chunk := range chunks {
		if len(chunk.Vector) == 0 {
			continue
		}
		if err := ensureVectorTable(ctx, tx, len(chunk.Vector)); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO chunks (event_id, ordinal, char_start, char_end) VALUES (?, ?, ?, ?)`,
			eventID, chunk.Ordinal, chunk.Start, chunk.End)
		if err != nil {
			return fmt.Errorf("insert chunk %d of event %q: %w", chunk.Ordinal, ev.UID, err)
		}
		chunkID, _ := result.LastInsertId()
		if err := insertVector(ctx, tx, chunkID, ev, chunk.Vector); err != nil {
			return err
		}
		if err := indexChunk(ctx, tx, chunkID, ev, chunk); err != nil {
			return err
		}
	}
	return nil
}

func insertVector(ctx context.Context, tx *sql.Tx, chunkID int64, ev event.Event, vector []float32) error {
	blob, err := sqlitevec.SerializeFloat32(vector)
	if err != nil {
		return fmt.Errorf("serialize embedding of event %q: %w", ev.UID, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chunk_embeddings (chunk_id, embedding, source, occurred_at) VALUES (?, ?, ?, ?)`,
		chunkID, blob, string(ev.Source), toUnixMillis(ev.Timestamp))
	if err != nil {
		return fmt.Errorf("insert embedding of event %q: %w", ev.UID, err)
	}
	return nil
}

// deleteChunks removes an event's chunks and their vectors, by the vec0
// primary key (a join would scan every vector).
func deleteChunks(ctx context.Context, tx *sql.Tx, eventID int64) error {
	ids, err := queryInts(ctx, tx, `SELECT id FROM chunks WHERE event_id = ?`, eventID)
	if err != nil {
		return fmt.Errorf("list chunks of event id %d: %w", eventID, err)
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE chunk_id = ?`, id); err != nil {
			return fmt.Errorf("delete vector of chunk %d: %w", id, err)
		}
		if err := unindexChunk(ctx, tx, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("delete chunks of event id %d: %w", eventID, err)
	}
	return nil
}

// ChunksFor returns each event's embedded chunks, in order, with vectors.
func (s *Store) ChunksFor(ctx context.Context, uids []string) (map[string][]storage.Chunk, error) {
	chunks := make(map[string][]storage.Chunk, len(uids))
	for _, uid := range uids {
		eventChunks, err := s.chunksOf(ctx, uid)
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
func (s *Store) chunksOf(ctx context.Context, uid string) ([]storage.Chunk, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chunks.id, ordinal, char_start, char_end FROM chunks
		JOIN events ON events.id = chunks.event_id WHERE events.uid = ? ORDER BY ordinal`, uid)
	if err != nil {
		return nil, fmt.Errorf("read chunks of event %q: %w", uid, err)
	}
	ids, chunks, err := scanChunks(rows)
	if err != nil {
		return nil, err
	}
	for i, id := range ids {
		if chunks[i].Vector, err = s.chunkVector(ctx, id); err != nil {
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

func (s *Store) chunkVector(ctx context.Context, chunkID int64) ([]float32, error) {
	var blob []byte
	if err := s.db.QueryRowContext(ctx, `SELECT embedding FROM chunk_embeddings WHERE chunk_id = ?`, chunkID).Scan(&blob); err != nil {
		return nil, fmt.Errorf("read vector of chunk %d: %w", chunkID, err)
	}
	return decodeFloat32s(blob)
}

// queryer is satisfied by both *sql.DB and *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
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
