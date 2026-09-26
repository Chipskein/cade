package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/chunking"
)

// splitIntoChunks (schema version 5) moves vectors from one per event to
// one per chunk. A short event is a single chunk covering its whole text,
// so its vector is still right and is copied as is; only long events (1.4%
// of a real 108k history) lose theirs and are left for `cade reindex`,
// which the pending mark makes ask and ingest mention.
func splitIntoChunks(ctx context.Context, tx *sql.Tx) error {
	statements := []string{createChunksTable, `CREATE INDEX chunks_event ON chunks (event_id)`}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create chunks: %w", err)
		}
	}
	dimensions, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(createVectorTableTemplate, dimensions)); err != nil {
		return fmt.Errorf("create chunk vectors with %d dimensions: %w", dimensions, err)
	}
	left, err := copySingleChunkVectors(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE event_embeddings`); err != nil {
		return fmt.Errorf("drop per-event vectors: %w", err)
	}
	return markReindexIfNeeded(ctx, tx, left)
}

type shortEvent struct {
	id, length, occurredAt int64
	source                 string
}

// copySingleChunkVectors turns each short event's vector into its only
// chunk and returns how many events with text were left without vectors.
func copySingleChunkVectors(ctx context.Context, tx *sql.Tx) (int, error) {
	events, left, err := eventsByLength(ctx, tx)
	if err != nil {
		return 0, err
	}
	for _, ev := range events {
		copied, err := copyVector(ctx, tx, ev)
		if err != nil {
			return 0, err
		}
		left += boolToInt(!copied)
	}
	return left, nil
}

// eventsByLength lists events with text that fit one chunk, and counts the
// longer ones. Lengths are in bytes, as chunking.Split measures them.
func eventsByLength(ctx context.Context, tx *sql.Tx) ([]shortEvent, int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, length(CAST(content AS BLOB)), occurred_at, source FROM events WHERE content != ''`)
	if err != nil {
		return nil, 0, fmt.Errorf("read event lengths: %w", err)
	}
	defer rows.Close()
	var short []shortEvent
	long := 0
	for rows.Next() {
		var ev shortEvent
		if err := rows.Scan(&ev.id, &ev.length, &ev.occurredAt, &ev.source); err != nil {
			return nil, 0, fmt.Errorf("scan event length: %w", err)
		}
		if ev.length > chunking.MaxChars {
			long++
			continue
		}
		short = append(short, ev)
	}
	return short, long, rows.Err()
}

func copyVector(ctx context.Context, tx *sql.Tx, ev shortEvent) (bool, error) {
	var blob []byte
	err := tx.QueryRowContext(ctx, `SELECT embedding FROM event_embeddings WHERE event_id = ?`, ev.id).Scan(&blob)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read vector of event id %d: %w", ev.id, err)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO chunks (event_id, ordinal, char_start, char_end) VALUES (?, 0, 0, ?)`, ev.id, ev.length)
	if err != nil {
		return false, fmt.Errorf("insert chunk of event id %d: %w", ev.id, err)
	}
	chunkID, _ := result.LastInsertId()
	_, err = tx.ExecContext(ctx, `INSERT INTO chunk_embeddings (chunk_id, embedding, source, occurred_at) VALUES (?, ?, ?, ?)`,
		chunkID, blob, ev.source, ev.occurredAt)
	if err != nil {
		return false, fmt.Errorf("copy vector of event id %d: %w", ev.id, err)
	}
	return true, nil
}

func markReindexIfNeeded(ctx context.Context, tx *sql.Tx, left int) error {
	if left == 0 {
		return nil
	}
	return putSetting(ctx, tx, reindexPendingSettingKey, "1")
}
