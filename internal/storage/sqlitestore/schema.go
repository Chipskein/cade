package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

const createEventsTable = `
CREATE TABLE IF NOT EXISTS events (
	id          INTEGER PRIMARY KEY,
	uid         TEXT    NOT NULL UNIQUE,
	occurred_at INTEGER NOT NULL, -- unix milliseconds, UTC
	source      TEXT    NOT NULL,
	content     TEXT    NOT NULL,
	metadata    TEXT    NOT NULL  -- JSON object
);
CREATE INDEX IF NOT EXISTS events_occurred_at ON events (occurred_at);
CREATE TABLE IF NOT EXISTS store_settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`

// source and occurred_at are vec0 metadata columns so the KNN search itself
// applies the filters (CA9.1); filtering after the top-k cut would silently
// drop matching events that ranked below k.
const createVectorTableTemplate = `
CREATE VIRTUAL TABLE IF NOT EXISTS event_embeddings USING vec0(
	event_id    INTEGER PRIMARY KEY,
	embedding   float[%d] distance_metric=cosine,
	source      TEXT,
	occurred_at INTEGER
)`

const dimensionsSettingKey = "embedding_dimensions"

func createSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, createEventsTable); err != nil {
		return fmt.Errorf("create events schema: %w", err)
	}
	return nil
}

// ensureVectorTable creates the vector table on first use, because its
// dimension is only known once the embedding model has produced a vector.
func ensureVectorTable(ctx context.Context, tx *sql.Tx, dimensions int) error {
	stored, found, err := storedDimensions(ctx, tx)
	if err != nil {
		return err
	}
	if found && stored != dimensions {
		return fmt.Errorf("embedding has %d dimensions, database expects %d (was a different embedding model configured?)", dimensions, stored)
	}
	if found {
		return nil
	}
	return createVectorTable(ctx, tx, dimensions)
}

func storedDimensions(ctx context.Context, querier queryRower) (int, bool, error) {
	var raw string
	row := querier.QueryRowContext(ctx, `SELECT value FROM store_settings WHERE key = ?`, dimensionsSettingKey)
	err := row.Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read embedding dimensions: %w", err)
	}
	dimensions, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, fmt.Errorf("stored embedding dimensions %q is not an integer: %w", raw, err)
	}
	return dimensions, true, nil
}

func createVectorTable(ctx context.Context, tx *sql.Tx, dimensions int) error {
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(createVectorTableTemplate, dimensions)); err != nil {
		return fmt.Errorf("create vector table with %d dimensions: %w", dimensions, err)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO store_settings (key, value) VALUES (?, ?)`,
		dimensionsSettingKey, strconv.Itoa(dimensions))
	if err != nil {
		return fmt.Errorf("record embedding dimensions: %w", err)
	}
	return nil
}

// queryRower is satisfied by both *sql.DB and *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
