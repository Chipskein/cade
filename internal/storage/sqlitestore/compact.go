package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/storage"
)

// vec0 has no xRename (sqlite-vec#43), so a compaction cannot build the
// new table beside the old one and swap names: it parks the live vectors
// and their metadata in these plain tables, recreates chunk_embeddings and
// copies them back.
const (
	vectorStagingTable   = "chunk_embeddings_staging"
	metadataStagingTable = "chunk_embeddings_staging_metadata"
)

// vectorLayout is one definition of chunk_embeddings: its CREATE template
// (%d is the dimension count) and its metadata columns.
type vectorLayout struct {
	create   string
	metadata string
}

var (
	// occurredAtLayout is schema versions 11 and 12: one date per chunk.
	occurredAtLayout = vectorLayout{create: createOccurredAtVectorTableTemplate, metadata: `source, occurred_at`}
	// dateRangeLayout is the layout since version 13.
	dateRangeLayout = vectorLayout{create: createVectorTableTemplate, metadata: `source, first_at, last_at`}
)

// vectorRewrite is how rewriteVectorTable carries each live vector over:
// a compaction copies it as stored, a format migration converts it.
type vectorRewrite struct {
	// componentBytes is the width of one stored component in the blocks.
	componentBytes int
	// convert turns a stored blob into the one written back.
	convert func(stored []byte) ([]byte, error)
	// readMetadata selects, from the old table, the new layout's metadata.
	readMetadata string
	layout       vectorLayout
}

// compactionRewrite keeps every vector and its metadata exactly as stored.
var compactionRewrite = vectorRewrite{componentBytes: int8Bytes, convert: keepVectorBlob,
	readMetadata: dateRangeLayout.metadata, layout: dateRangeLayout}

func keepVectorBlob(stored []byte) ([]byte, error) {
	return stored, nil
}

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
	err = s.inTransaction(ctx, func(tx *sql.Tx) error { return rewriteVectorTable(ctx, tx, dimensions, compactionRewrite) })
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
func rewriteVectorTable(ctx context.Context, tx *sql.Tx, dimensions int, rewrite vectorRewrite) error {
	if err := stageVectors(ctx, tx, dimensions, rewrite); err != nil {
		return err
	}
	// vec_int8 reads the staged blobs back as int8; vec0 reads a bare blob
	// as float32.
	return execEach(ctx, tx, dimensions,
		`DROP TABLE chunk_embeddings`,
		fmt.Sprintf(rewrite.layout.create, dimensions),
		`INSERT INTO chunk_embeddings (chunk_id, embedding, `+rewrite.layout.metadata+`) SELECT chunk_id, vec_int8(embedding), `+
			rewrite.layout.metadata+` FROM `+metadataStagingTable+` JOIN `+vectorStagingTable+` USING (chunk_id) ORDER BY chunk_id`,
		`DROP TABLE `+vectorStagingTable,
		`DROP TABLE `+metadataStagingTable)
}

// stageVectors reads the metadata columns through vec0, which is fast,
// and the vectors from its blocks (compact_blocks.go), which is not. Both
// staging tables are keyed by chunk_id, so the copy back is ordered by a
// rowid scan: a sort would drop the subtype vec_int8 marks its result with,
// and vec0 would read the blob as float32.
func stageVectors(ctx context.Context, tx *sql.Tx, dimensions int, rewrite vectorRewrite) error {
	err := execEach(ctx, tx, dimensions,
		`CREATE TABLE `+metadataStagingTable+` (chunk_id INTEGER PRIMARY KEY, `+rewrite.layout.metadata+`)`,
		`INSERT INTO `+metadataStagingTable+` SELECT chunk_id, `+rewrite.readMetadata+` FROM chunk_embeddings`,
		`CREATE TABLE `+vectorStagingTable+` (chunk_id INTEGER PRIMARY KEY, embedding BLOB NOT NULL)`)
	if err != nil {
		return err
	}
	slots, err := liveVectorSlots(ctx, tx)
	if err != nil {
		return err
	}
	return copyLiveVectors(ctx, tx, slots, dimensions, rewrite)
}

func execEach(ctx context.Context, tx *sql.Tx, dimensions int, statements ...string) error {
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("compact vectors (%d dimensions), at %q: %w", dimensions, statement, err)
		}
	}
	return nil
}
