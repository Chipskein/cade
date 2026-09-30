package testfakes

import (
	"context"
	"maps"
	"slices"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// FakeEventBatch writes straight into its FakeEventStore (whose
// EventWriter methods it borrows) and, on Rollback, puts back what the
// store held when the batch began, as the real transaction would.
type FakeEventBatch struct {
	*FakeEventStore
	began    fakeStoreContents
	finished bool
}

var _ storage.EventBatch = (*FakeEventBatch)(nil)

// fakeStoreContents is what a batch can change in the fake store.
type fakeStoreContents struct {
	events     []event.Event
	chunks     map[string][]storage.Chunk
	embeddings map[string][]float32
	updated    []string
}

// BeginBatch opens a FakeEventBatch and counts it in BatchesBegun.
func (f *FakeEventStore) BeginBatch(context.Context) (storage.EventBatch, error) {
	f.BatchesBegun++
	return &FakeEventBatch{FakeEventStore: f, began: f.contents()}, nil
}

func (f *FakeEventStore) contents() fakeStoreContents {
	return fakeStoreContents{
		events: slices.Clone(f.Events), chunks: maps.Clone(f.Chunks),
		embeddings: maps.Clone(f.Embeddings), updated: slices.Clone(f.Updated),
	}
}

// Commit keeps the batch's writes and counts it in Commits; it fails with
// FailWith, like every other fake write.
func (b *FakeEventBatch) Commit() error {
	if b.FailWith != nil {
		return b.FailWith
	}
	b.finished = true
	b.Commits++
	return nil
}

// Rollback restores the store as it was at BeginBatch, unless committed.
func (b *FakeEventBatch) Rollback() error {
	if b.finished {
		return nil
	}
	b.finished = true
	b.Events, b.Chunks, b.Embeddings, b.Updated = b.began.events, b.began.chunks, b.began.embeddings, b.began.updated
	b.Rollbacks++
	return nil
}
