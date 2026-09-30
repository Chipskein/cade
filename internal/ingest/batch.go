package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/chipskein/cade/internal/storage"
)

// eventsPerCommit is how many collected events share one transaction
// (issue #51): a commit per event wrote ~28× the database's size on a
// first ingestion. docs/BENCHMARKS.md has the measurement behind it.
const eventsPerCommit = 200

// batchMaxAge commits a batch early when embedding is slow (the CPU), so
// it stays open for less than the store's 5 s busy timeout and a `cade
// forget` run meanwhile waits for the commit instead of failing.
const batchMaxAge = 2 * time.Second

// eventBatcher opens a storage.EventBatch on demand and commits it when
// full, when old, and at the end of the collection. An interruption rolls
// back only the open batch; the next run stores it again (RF1.5).
type eventBatcher struct {
	store    storage.EventStore
	now      func() time.Time
	size     int
	open     storage.EventBatch
	events   int
	openedAt time.Time
}

func newEventBatcher(store storage.EventStore, now func() time.Time, size int) *eventBatcher {
	return &eventBatcher{store: store, now: now, size: size}
}

// writer returns the open batch, opening one if needed.
func (b *eventBatcher) writer(ctx context.Context) (storage.EventWriter, error) {
	if b.open != nil {
		return b.open, nil
	}
	batch, err := b.store.BeginBatch(ctx)
	if err != nil {
		return nil, err
	}
	b.open, b.events, b.openedAt = batch, 0, b.now()
	return batch, nil
}

// eventWritten counts an event done and commits a full or old batch.
func (b *eventBatcher) eventWritten() error {
	b.events++
	if b.events < b.size && b.now().Sub(b.openedAt) < batchMaxAge {
		return nil
	}
	return b.commit()
}

// commit is the one point where the pipeline's writes become durable; a
// stop request without a signal (#54, Windows) belongs here too.
func (b *eventBatcher) commit() error {
	batch := b.take()
	if batch == nil {
		return nil
	}
	if err := batch.Commit(); err != nil {
		return errors.Join(err, batch.Rollback())
	}
	return nil
}

// finish commits after a complete collection and rolls back after a
// failed one: the failed event may be half written.
func (b *eventBatcher) finish(collectErr error) error {
	if collectErr == nil {
		return b.commit()
	}
	batch := b.take()
	if batch == nil {
		return collectErr
	}
	return errors.Join(collectErr, batch.Rollback())
}

// take hands over the open batch, if any, leaving none open.
func (b *eventBatcher) take() storage.EventBatch {
	batch := b.open
	b.open = nil
	return batch
}
