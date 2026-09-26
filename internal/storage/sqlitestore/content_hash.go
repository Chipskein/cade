package sqlitestore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/chipskein/cade/internal/storage"
)

// contentHash identifies an event's text. Events with the same text have
// the same vector: a page visited 15 times, a message cached twice.
func contentHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

// addContentHash (schema version 3) adds and fills events.content_hash,
// so ingestion can reuse a vector instead of embedding the same text
// again. Derived data only: nothing the user stored is rewritten.
func addContentHash(ctx context.Context, tx *sql.Tx) error {
	statements := []string{`ALTER TABLE events ADD COLUMN content_hash TEXT`, `CREATE INDEX events_content_hash ON events (content_hash)`}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("add content_hash: %w", err)
		}
	}
	hashes, err := contentHashes(ctx, tx)
	if err != nil {
		return err
	}
	for id, hash := range hashes {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET content_hash = ? WHERE id = ?`, hash, id); err != nil {
			return fmt.Errorf("store content_hash of event id %d: %w", id, err)
		}
	}
	return nil
}

// contentHashes reads every event first: the single connection cannot
// write while a result set is open.
func contentHashes(ctx context.Context, tx *sql.Tx) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, content FROM events`)
	if err != nil {
		return nil, fmt.Errorf("read events to hash: %w", err)
	}
	defer rows.Close()
	hashes := map[int64]string{}
	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, fmt.Errorf("scan event to hash: %w", err)
		}
		hashes[id] = contentHash(content)
	}
	return hashes, rows.Err()
}

// reuseCandidates bounds the events checked for a vector to reuse; any
// one of them with a vector will do.
const reuseCandidates = 8

// StoredChunksForContent returns the embedded chunks of a stored event
// with the same text, if one has them: same text, same chunks and vectors.
func (s *Store) StoredChunksForContent(ctx context.Context, content string) ([]storage.Chunk, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uid FROM events WHERE content_hash = ? LIMIT ?`, contentHash(content), reuseCandidates)
	if err != nil {
		return nil, false, fmt.Errorf("find events with the same content: %w", err)
	}
	uids, err := collectStrings(rows)
	if err != nil {
		return nil, false, err
	}
	for _, uid := range uids {
		chunks, err := s.chunksOf(ctx, uid)
		if err != nil || len(chunks) > 0 {
			return chunks, len(chunks) > 0, err
		}
	}
	return nil, false, nil
}

func collectStrings(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
