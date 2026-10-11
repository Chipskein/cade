package testfakes

import (
	"context"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

// The fake filters with listing.Select, the in-memory reference the real
// store's SQL is tested against.

// CountMatching counts the events EventsMatching would return, up to upTo.
func (f *FakeEventStore) CountMatching(ctx context.Context, filter storage.EventFilter, upTo int) (int, error) {
	events, err := f.EventsMatching(ctx, filter)
	return min(len(events), upTo), err
}

// EventsMatching returns the events in the filter's period, source and
// folder that listing.Select and the graph filter keep, oldest first.
func (f *FakeEventStore) EventsMatching(ctx context.Context, filter storage.EventFilter) ([]event.Event, error) {
	to := filter.To
	if to.IsZero() {
		to = time.Unix(1<<40, 0) // a zero end is unbounded, as in SQL
	}
	inPeriod, err := f.EventsBetween(ctx, filter.From, to)
	var scoped []event.Event
	for _, ev := range inPeriod {
		if (filter.Source == "" || ev.Source == filter.Source) && filter.Graph.Keeps(ev, storage.GraphLinks{EntityIDs: f.linkedIDs(ev)}) {
			scoped = append(scoped, ev)
		}
	}
	return listing.Select(listing.InFolder(scoped, filter.Folder), filter.Direction, filter.People), err
}

// linkedIDs are the ids of the entities ev refers to, as EntitiesOfEvent
// gives them. Like EntitiesOfEvent, the fake derives links from each
// event alone, so it does not link a message to the image it attaches.
func (f *FakeEventStore) linkedIDs(ev event.Event) []int64 {
	var ids []int64
	for _, ref := range entity.RefsOf(ev) {
		ids = append(ids, f.storedEntity(ref).ID)
	}
	return ids
}

// keepAmong drops the hits whose event does not satisfy among.
func (f *FakeEventStore) keepAmong(hits []storage.ScoredEvent, among *storage.EventFilter) []storage.ScoredEvent {
	if among == nil {
		return hits
	}
	matching, _ := f.EventsMatching(context.Background(), *among)
	allowed := map[string]bool{}
	for _, ev := range matching {
		allowed[ev.UID] = true
	}
	var kept []storage.ScoredEvent
	for _, hit := range hits {
		if allowed[hit.Event.UID] {
			kept = append(kept, hit)
		}
	}
	return kept
}
