package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
)

// Reading the embedding column through vec0 opens its whole block (3 MB at
// 768 dimensions) once per row: 57 s for the reference machine's 416 k
// vectors. Reading each block once from vec0's shadow tables
// (sqlite-vec v0.1.6 layout) and slicing the live vectors out avoids that;
// TestCompactVectorsKeepsTheSameNearest fails if the layout changes.

// float32Bytes is the width of one vector component in the blocks.
const float32Bytes = 4

// vectorSlot is where a live vector sits: chunk_embeddings_rowids maps the
// vec0 primary key to a block (its chunk_id) and a position in it.
type vectorSlot struct {
	chunkID, block, offset int64
}

func liveVectorSlots(ctx context.Context, tx *sql.Tx) ([]vectorSlot, error) {
	rows, err := tx.QueryContext(ctx, `SELECT rowid, chunk_id, chunk_offset FROM chunk_embeddings_rowids ORDER BY chunk_id, chunk_offset`)
	if err != nil {
		return nil, fmt.Errorf("list vector positions: %w", err)
	}
	defer rows.Close()
	var slots []vectorSlot
	for rows.Next() {
		var slot vectorSlot
		if err := rows.Scan(&slot.chunkID, &slot.block, &slot.offset); err != nil {
			return nil, fmt.Errorf("read vector position: %w", err)
		}
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

// copyLiveVectors stages each slot's vector, converted; slots come sorted
// by block, so each block is read once.
func copyLiveVectors(ctx context.Context, tx *sql.Tx, slots []vectorSlot, dimensions int, rewrite vectorRewrite) error {
	insert, err := tx.PrepareContext(ctx, `INSERT INTO `+vectorStagingTable+` (chunk_id, embedding) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare vector staging: %w", err)
	}
	defer insert.Close()
	blocks := vectorBlockReader{tx: tx, block: -1}
	for _, slot := range slots {
		vector, err := blocks.vector(ctx, slot, dimensions*rewrite.componentBytes)
		if err != nil {
			return err
		}
		if err := stageVector(ctx, insert, slot, vector, rewrite); err != nil {
			return err
		}
	}
	return nil
}

func stageVector(ctx context.Context, insert *sql.Stmt, slot vectorSlot, stored []byte, rewrite vectorRewrite) error {
	converted, err := rewrite.convert(stored)
	if err != nil {
		return fmt.Errorf("convert vector of chunk %d: %w", slot.chunkID, err)
	}
	if _, err := insert.ExecContext(ctx, slot.chunkID, converted); err != nil {
		return fmt.Errorf("stage vector of chunk %d: %w", slot.chunkID, err)
	}
	return nil
}

// vectorBlockReader keeps the last block read.
type vectorBlockReader struct {
	tx    *sql.Tx
	block int64
	blob  []byte
}

func (r *vectorBlockReader) vector(ctx context.Context, slot vectorSlot, width int) ([]byte, error) {
	if slot.block != r.block {
		if err := r.load(ctx, slot.block); err != nil {
			return nil, err
		}
	}
	start := int(slot.offset) * width
	if start+width > len(r.blob) {
		return nil, fmt.Errorf("vector of chunk %d at position %d of block %d: block has %d bytes, expected at least %d", slot.chunkID, slot.offset, slot.block, len(r.blob), start+width)
	}
	return r.blob[start : start+width], nil
}

func (r *vectorBlockReader) load(ctx context.Context, block int64) error {
	err := r.tx.QueryRowContext(ctx, `SELECT vectors FROM chunk_embeddings_vector_chunks00 WHERE rowid = ?`, block).Scan(&r.blob)
	if err != nil {
		return fmt.Errorf("read vector block %d: %w", block, err)
	}
	r.block = block
	return nil
}
