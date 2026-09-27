// Package listing applies exact criteria — message direction and people —
// to events. Similarity search cannot enforce "from Ana" or "that I
// received": names and direction barely move an embedding, and top-k would
// cut matching events anyway.
package listing

import (
	"slices"

	"github.com/chipskein/cade/internal/event"
)

// Direction filters Teams messages by who sent them.
type Direction int

const (
	AnyDirection Direction = iota
	Received
	Sent
)

// Criteria restricts events to a direction and to people.
type Criteria struct {
	Direction Direction
	People    []string
}

// IsEmpty reports whether the criteria restrict nothing.
func (c Criteria) IsEmpty() bool {
	return c.Direction == AnyDirection && len(c.People) == 0
}

// PersonProbe reports whether some event in scope matches person: a
// search of events in memory, or a query to the store.
type PersonProbe func(person PersonMatcher) (bool, error)

// Resolution is the criteria's people as they appear in the events.
type Resolution struct {
	People []PersonMatcher
	// Matched are the resolved names; Unknown the names that matched
	// nobody (a misread name, a company), dropped rather than emptying
	// the result, so the user can be told.
	Matched []string
	Unknown []string
}

// Resolve finds how each person appears, asking exists about the full
// name and then the first word.
//
//	resolution, err := criteria.Resolve(func(p listing.PersonMatcher) (bool, error) { return store.Has(p) })
func (c Criteria) Resolve(exists PersonProbe) (Resolution, error) {
	var resolution Resolution
	for _, name := range c.People {
		person, found, err := resolvePerson(name, c.Direction, exists)
		if err != nil {
			return Resolution{}, err
		}
		if !found {
			resolution.Unknown = append(resolution.Unknown, name)
			continue
		}
		resolution.People, resolution.Matched = append(resolution.People, person), append(resolution.Matched, person.Name)
	}
	return resolution, nil
}

// Apply keeps events matching the direction and any of the people,
// resolving the names among the events themselves.
//
//	kept, matched, unknown := listing.Criteria{People: []string{"Ana"}}.Apply(events)
func (c Criteria) Apply(events []event.Event) ([]event.Event, []string, []string) {
	directed := c.Direction.keep(events)
	inDirected := func(person PersonMatcher) (bool, error) { return slices.ContainsFunc(directed, person.Matches), nil }
	resolution, _ := c.Resolve(inDirected)
	return keepPeople(directed, resolution.People), resolution.Matched, resolution.Unknown
}

// Select keeps the events matching direction and any of people (every
// event when there are none). The store runs the same filter in SQL;
// this is its in-memory reference.
func Select(events []event.Event, direction Direction, people []PersonMatcher) []event.Event {
	return keepPeople(direction.keep(events), people)
}

func keepPeople(events []event.Event, people []PersonMatcher) []event.Event {
	if len(people) == 0 {
		return events
	}
	return filterEvents(events, func(ev event.Event) bool { return matchesAnyPerson(ev, people) })
}

// keep applies the direction; for received messages, it also drops a
// group message mentioning only other people, which was not addressed to
// the user. Who is who is learned from events themselves.
func (d Direction) keep(events []event.Event) []event.Event {
	if d != Received {
		return filterEvents(events, d.keepMessage)
	}
	known := addresseesOf(events)
	return filterEvents(events, func(ev event.Event) bool {
		sentToOthers := ev.Source == event.SourceTeams && known.addressedToOthers(ev)
		return d.keepMessage(ev) && !sentToOthers
	})
}

// keepMessage applies the direction to Teams messages; other events
// always pass. "Received" excludes channel posts, which are published to a
// team rather than sent to the user.
func (d Direction) keepMessage(ev event.Event) bool {
	return d == AnyDirection || ev.Source != event.SourceTeams || IndexedDirection(ev) == d.Indexed()
}

func filterEvents(events []event.Event, keep func(event.Event) bool) []event.Event {
	var kept []event.Event
	for _, ev := range events {
		if keep(ev) {
			kept = append(kept, ev)
		}
	}
	return kept
}
