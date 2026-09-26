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
	LastQuery     storage.SimilarityQuery
	FailWith      error
	Closed        bool
}

// NewFakeEventStore returns an empty store.
func NewFakeEventStore() *FakeEventStore {
	return &FakeEventStore{Embeddings: map[string][]float32{}}
}

func (f *FakeEventStore) HasEvent(_ context.Context, uid string) (bool, error) {
	for _, ev := range f.Events {
		if ev.UID == uid {
			return true, f.FailWith
		}
	}
	return false, f.FailWith
}

func (f *FakeEventStore) SaveEvent(ctx context.Context, ev event.Event, embedding []float32) (bool, error) {
	known, _ := f.HasEvent(ctx, ev.UID)
	if known || f.FailWith != nil {
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
