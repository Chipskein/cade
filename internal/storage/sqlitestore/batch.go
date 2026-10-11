package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// eventBatch runs every read and write on one open transaction: the store
// has a single connection, so a read outside it would wait for the commit
// forever.
type eventBatch struct {
	tx *sql.Tx
}

var _ storage.EventBatch = (*eventBatch)(nil)

// BeginBatch opens a transaction for many events (issue #51). A cancelled
// ctx rolls it back, so an interrupted ingestion loses only this batch,
// which the next run stores again.
//
//	batch, err := store.BeginBatch(ctx)
//	defer batch.Rollback()
//	... batch.SaveEvent(ctx, ev, chunks) ...
//	err = batch.Commit()
func (s *Store) BeginBatch(ctx context.Context) (storage.EventBatch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin event batch: %w", err)
	}
	return &eventBatch{tx: tx}, nil
}

func (b *eventBatch) StoredEvent(ctx context.Context, uid string) (event.Event, bool, error) {
	return storedEvent(ctx, b.tx, uid)
}

func (b *eventBatch) StoredChunksForContent(ctx context.Context, content string) ([]storage.Chunk, bool, error) {
	return storedChunksForContent(ctx, b.tx, content)
}

func (b *eventBatch) UpdateEvent(ctx context.Context, ev event.Event, chunks []storage.Chunk) error {
	return updateEventIn(ctx, b.tx, ev, chunks)
}

func (b *eventBatch) SaveEvent(ctx context.Context, ev event.Event, chunks []storage.Chunk) (bool, error) {
	return saveEventIn(ctx, b.tx, ev, chunks)
}

func (b *eventBatch) IsForgotten(ctx context.Context, uid string) (bool, error) {
	return isForgotten(ctx, b.tx, uid)
}

func (b *eventBatch) Commit() error {
	if err := b.tx.Commit(); err != nil {
		return fmt.Errorf("commit event batch: %w", err)
	}
	return nil
}

// Rollback ignores a transaction already finished by Commit or by its
// context being cancelled.
func (b *eventBatch) Rollback() error {
	if err := b.tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("roll back event batch: %w", err)
	}
	return nil
}
