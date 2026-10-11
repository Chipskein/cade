package storage

import (
	"context"

	"github.com/chipskein/cade/internal/event"
)

// PersonIndex finds everything about one person, for `forget --person` and
// `export --person` (#86); listing.PersonNames and listing.EventsOfPerson
// are its in-memory reference.
type PersonIndex interface {
	// PeopleNamed lists the full name keys of the senders and authors a
	// typed name matches as whole words.
	PeopleNamed(ctx context.Context, name string) ([]string, error)
	// EventsOfPerson returns the events about the person with that full
	// name key, oldest first.
	EventsOfPerson(ctx context.Context, key string) ([]event.Event, error)
}
