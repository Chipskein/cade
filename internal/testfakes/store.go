// Package testfakes holds named in-memory stand-ins for the storage and
// model boundaries, shared by tests across packages.
package testfakes

import (
	"context"
	"sort"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// FakeEventStore is an in-memory storage.EventStore. SearchSimilar returns
// the canned SearchResults (filtered like the real store) and records the
// last query.
type FakeEventStore struct {
	Events        []event.Event
	Embeddings    map[string][]float32
	SearchResults []storage.ScoredEvent
	// Updated lists the UIDs passed to UpdateEvent, in order.
	Updated   []string
	LastQuery storage.SimilarityQuery
	FailWith  error
	Closed    bool
	// EmbeddingModelName, Pending and ReindexStarted back the
	// storage.EmbeddingIndex methods.
	EmbeddingModelName string
	Pending            bool
	ReindexStarted     int
	// Modifications backs FileModificationsBetween; MarkMissingFiles
	// records its roots and paths and reports MissingFiles as removed.
	Modifications []storage.FileModification
	MarkedRoots   []string
	LastPresent   map[string]bool
	MissingFiles  int
}

// NewFakeEventStore returns an empty store.
func NewFakeEventStore() *FakeEventStore {
	return &FakeEventStore{Embeddings: map[string][]float32{}}
}

func (f *FakeEventStore) StoredEvent(_ context.Context, uid string) (event.Event, bool, error) {
	index := f.indexOf(uid)
	if index < 0 {
		return event.Event{}, false, f.FailWith
	}
	return f.Events[index], true, f.FailWith
}

func (f *FakeEventStore) indexOf(uid string) int {
	for i, ev := range f.Events {
		if ev.UID == uid {
			return i
		}
	}
	return -1
}

// UpdateEvent replaces the event and records the UID in Updated.
func (f *FakeEventStore) UpdateEvent(_ context.Context, ev event.Event, embedding []float32) error {
	index := f.indexOf(ev.UID)
	if index < 0 || f.FailWith != nil {
		return f.FailWith
	}
	f.Events[index], f.Embeddings[ev.UID] = ev, embedding
	f.Updated = append(f.Updated, ev.UID)
	return nil
}

func (f *FakeEventStore) SaveEvent(_ context.Context, ev event.Event, embedding []float32) (bool, error) {
	if f.indexOf(ev.UID) >= 0 || f.FailWith != nil {
		return false, f.FailWith
	}
	f.Events = append(f.Events, ev)
	f.Embeddings[ev.UID] = embedding
	return true, nil
}

func (f *FakeEventStore) EventsBetween(_ context.Context, from, to time.Time) ([]event.Event, error) {
	var matching []event.Event
	for _, ev := range f.Events {
		if !ev.Timestamp.Before(from) && ev.Timestamp.Before(to) {
			matching = append(matching, ev)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool { return matching[i].Timestamp.Before(matching[j].Timestamp) })
	return matching, f.FailWith
}

func (f *FakeEventStore) SearchSimilar(_ context.Context, query storage.SimilarityQuery) ([]storage.ScoredEvent, error) {
	f.LastQuery = query
	var hits []storage.ScoredEvent
	for _, hit := range f.SearchResults {
		if len(hits) < query.Limit && (query.Source == "" || hit.Event.Source == query.Source) {
			hits = append(hits, hit)
		}
	}
	return hits, f.FailWith
}

func (f *FakeEventStore) DeleteSource(_ context.Context, source event.Source) (int, error) {
	var kept []event.Event
	for _, ev := range f.Events {
		if ev.Source != source {
			kept = append(kept, ev)
		}
	}
	removed := len(f.Events) - len(kept)
	f.Events = kept
	return removed, f.FailWith
}

func (f *FakeEventStore) Close() error {
	f.Closed = true
	return nil
}

func (f *FakeEventStore) EmbeddingsFor(_ context.Context, uids []string) (map[string][]float32, error) {
	found := map[string][]float32{}
	for _, uid := range uids {
		if vector := f.Embeddings[uid]; vector != nil {
			found[uid] = vector
		}
	}
	return found, f.FailWith
}

// Embedding index: the fake records the model and a pending rebuild, and
// treats events with text and no entry in Embeddings as missing a vector.

func (f *FakeEventStore) EmbeddingModel(context.Context) (string, error) {
	return f.EmbeddingModelName, f.FailWith
}

func (f *FakeEventStore) RecordEmbeddingModel(_ context.Context, model string) error {
	f.EmbeddingModelName = model
	return f.FailWith
}

func (f *FakeEventStore) StartReindex(_ context.Context, model string) error {
	f.Embeddings, f.EmbeddingModelName, f.ReindexStarted, f.Pending = map[string][]float32{}, model, f.ReindexStarted+1, true
	return f.FailWith
}

func (f *FakeEventStore) ReindexPending(context.Context) (bool, error) {
	return f.Pending, f.FailWith
}

func (f *FakeEventStore) EventsWithoutEmbedding(_ context.Context, limit int) ([]event.Event, error) {
	missing := f.missingEmbeddings()
	return missing[:min(limit, len(missing))], f.FailWith
}

func (f *FakeEventStore) CountEventsWithoutEmbedding(context.Context) (int, error) {
	return len(f.missingEmbeddings()), f.FailWith
}

func (f *FakeEventStore) missingEmbeddings() []event.Event {
	var missing []event.Event
	for _, ev := range f.Events {
		if ev.Content != "" && f.Embeddings[ev.UID] == nil {
			missing = append(missing, ev)
		}
	}
	return missing
}

func (f *FakeEventStore) SaveEmbeddings(_ context.Context, embeddings []storage.EventEmbedding) error {
	if f.FailWith != nil {
		return f.FailWith
	}
	for _, pair := range embeddings {
		f.Embeddings[pair.Event.UID] = pair.Vector
	}
	return nil
}

func (f *FakeEventStore) FinishReindex(context.Context) error {
	f.Pending = false
	return f.FailWith
}

// StoredEmbeddingForContent returns the embedding of an event with the
// same content, as the real store does by content hash.
func (f *FakeEventStore) StoredEmbeddingForContent(_ context.Context, content string) ([]float32, bool, error) {
	for _, ev := range f.Events {
		if vector := f.Embeddings[ev.UID]; ev.Content == content && vector != nil {
			return vector, true, f.FailWith
		}
	}
	return nil, false, f.FailWith
}

// FileModificationsBetween returns Modifications within [from, to).
func (f *FakeEventStore) FileModificationsBetween(_ context.Context, from, to time.Time) ([]storage.FileModification, error) {
	var kept []storage.FileModification
	for _, modification := range f.Modifications {
		if !modification.ModifiedAt.Before(from) && modification.ModifiedAt.Before(to) {
			kept = append(kept, modification)
		}
	}
	return kept, f.FailWith
}

// MarkMissingFiles records the call and reports MissingFiles as removed.
func (f *FakeEventStore) MarkMissingFiles(_ context.Context, root string, present map[string]bool, _ time.Time) (int, error) {
	f.MarkedRoots, f.LastPresent = append(f.MarkedRoots, root), present
	return f.MissingFiles, f.FailWith
}
