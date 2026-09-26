package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/event"
)

// DeleteSource removes the events of source and their embeddings in one
// transaction, so an interruption never leaves embeddings without events,
// then compacts the file so the deleted text leaves the disk.
func (s *Store) DeleteSource(ctx context.Context, source event.Source) (int, error) {
	removed, err := s.deleteSourceRows(ctx, source)
	if err != nil {
		return 0, err
	}
	return removed, s.compact(ctx)
}

func (s *Store) deleteSourceRows(ctx context.Context, source event.Source) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := deleteSourceEmbeddings(ctx, tx, source); err != nil {
		return 0, err
	}
	if err := deleteSourceHistory(ctx, tx, source); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM events WHERE source = ?`, string(source))
	if err != nil {
		return 0, fmt.Errorf("delete %s events: %w", source, err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(removed), tx.Commit()
}

// deleteSourceEmbeddings deletes through the vec0 metadata column, which
// avoids a subquery join that vec0 cannot plan.
func deleteSourceEmbeddings(ctx context.Context, tx *sql.Tx, source event.Source) error {
	_, found, err := storedDimensions(ctx, tx)
	if err != nil || !found {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE source = ?`, string(source)); err != nil {
		return fmt.Errorf("delete %s embeddings: %w", source, err)
	}
	return nil
}

// deleteSourceHistory removes what a source keeps outside events: the
// file edit history holds paths, which `forget file` must erase too.
func deleteSourceHistory(ctx context.Context, tx *sql.Tx, source event.Source) error {
	if source != event.SourceFile {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM file_modifications`); err != nil {
		return fmt.Errorf("delete file modifications: %w", err)
	}
	return nil
}
