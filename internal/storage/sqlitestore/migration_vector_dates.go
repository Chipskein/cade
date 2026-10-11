package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// spreadVectorDates (schema version 13) replaces each vector's date with
// the first and last dates of its chunk's events, so a chunk shared by
// events months apart still answers a period filter inside the KNN (#67).
// Every chunk still has one event here, so both are its date. The vectors
// are copied as stored; rebuilding them takes hours of GPU: the step
// backs up.
func spreadVectorDates(ctx context.Context, tx *sql.Tx) error {
	dimensions, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	if converted, err := vectorTableLacks(ctx, tx, `occurred_at`); err != nil || converted {
		return err
	}
	return rewriteVectorTable(ctx, tx, dimensions, dateSpreadRewrite)
}

// vectorTableLacks reports whether chunk_embeddings is declared without
// fragment: a step that finds its layout already in place has nothing to
// convert (a table created by this version, then marked older).
func vectorTableLacks(ctx context.Context, tx *sql.Tx, fragment string) (bool, error) {
	var declaration string
	err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = 'chunk_embeddings'`).Scan(&declaration)
	if err != nil {
		return false, fmt.Errorf("read the declaration of chunk_embeddings, expected the table to exist: %w", err)
	}
	return !strings.Contains(declaration, fragment), nil
}

// dateSpreadRewrite reads the version 11 layout and writes the current one.
var dateSpreadRewrite = vectorRewrite{componentBytes: int8Bytes, convert: keepVectorBlob,
	readMetadata: `source, occurred_at, occurred_at`, layout: dateRangeLayout}
