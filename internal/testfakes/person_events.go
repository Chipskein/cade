package testfakes

import (
	"context"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.PersonIndex = (*FakeEventStore)(nil)

// PeopleNamed applies listing.PersonNames, the store's reference.
func (f *FakeEventStore) PeopleNamed(_ context.Context, name string) ([]string, error) {
	return listing.PersonNames(f.Events, name), f.FailWith
}

// EventsOfPerson applies listing.EventsOfPerson, oldest first.
func (f *FakeEventStore) EventsOfPerson(ctx context.Context, key string) ([]event.Event, error) {
	all, err := f.EventsMatching(ctx, storage.EventFilter{})
	return listing.EventsOfPerson(all, key), err
}
