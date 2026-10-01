package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/storage"
)

// vec0 has no xRename (sqlite-vec#43), so a compaction cannot build the
// new table beside the old one and swap names: it parks the live vectors
// in this plain table, recreates chunk_embeddings and copies them back.
const vectorStagingTable = "chunk_embeddings_staging"

const vectorColumns = `chunk_id, embedding, source, occurred_at`

// vectorSlotsQuery reads vec0's shadow tables: each block row records its
// size in positions, and each live vector has one rowids row.
const vectorSlotsQuery = `SELECT
	(SELECT COALESCE(SUM(size), 0) FROM chunk_embeddings_chunks),
	(SELECT COUNT(*) FROM chunk_embeddings_rowids)`

// VectorSlots measures how full the vector blocks are; zero before the
// first vector creates the table.
//
//	slots, err := store.VectorSlots(ctx) // slots.EmptyShare() > 0.2: compact
func (s *Store) VectorSlots(ctx context.Context) (storage.VectorSlots, error) {
	return vectorSlots(ctx, s.db)
}

func vectorSlots(ctx context.Context, querier queryRower) (storage.VectorSlots, error) {
	_, found, err := storedDimensions(ctx, querier)
	if err != nil || !found {
		return storage.VectorSlots{}, err
	}
	var slots storage.VectorSlots
	if err := querier.QueryRowContext(ctx, vectorSlotsQuery).Scan(&slots.Slots, &slots.Vectors); err != nil {
		return slots, fmt.Errorf("measure vector blocks: %w", err)
	}
	return slots, nil
}

// CompactVectors rewrites chunk_embeddings with only the live vectors, in
// chunk order, then VACUUMs so the file shrinks. The vectors and the
// search results stay the same; the rewrite needs free disk for a copy of
// the vectors. It returns the blocks as they are afterwards.
//
//	after, err := store.CompactVectors(ctx) // after.Slots == ceil(after.Vectors/1024)*1024
func (s *Store) CompactVectors(ctx context.Context) (storage.VectorSlots, error) {
	dimensions, found, err := storedDimensions(ctx, s.db)
	if err != nil || !found {
		return storage.VectorSlots{}, err
	}
	err = s.inTransaction(ctx, func(tx *sql.Tx) error { return rewriteVectorTable(ctx, tx, dimensions) })
	if err != nil {
		return storage.VectorSlots{}, err
	}
	if err := s.compact(ctx); err != nil {
		return storage.VectorSlots{}, err
	}
	return s.VectorSlots(ctx)
}

// rewriteVectorTable runs in one transaction, so an interruption leaves
// the old table intact.
func rewriteVectorTable(ctx context.Context, tx *sql.Tx, dimensions int) error {
	statements := []string{
		`CREATE TABLE ` + vectorStagingTable + ` AS SELECT ` + vectorColumns + ` FROM chunk_embeddings`,
		`DROP TABLE chunk_embeddings`,
		fmt.Sprintf(createVectorTableTemplate, dimensions),
		`INSERT INTO chunk_embeddings (` + vectorColumns + `) SELECT ` + vectorColumns + ` FROM ` + vectorStagingTable + ` ORDER BY chunk_id`,
		`DROP TABLE ` + vectorStagingTable,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("compact vectors (%d dimensions), at %q: %w", dimensions, statement, err)
		}
	}
	return nil
}
