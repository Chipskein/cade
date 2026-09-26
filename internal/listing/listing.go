// Package listing applies exact criteria — message direction and people —
// to events. Similarity search cannot enforce "from Ana" or "that I
// received": names and direction barely move an embedding, and top-k would
// cut matching events anyway.
package listing

import (
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

// Apply keeps events matching the direction and any of the people. People
// that match nobody (a misread name, a company) are dropped rather than
// emptying the result, and returned so the user can be told.
//
//	kept, matched, unknown := listing.Criteria{People: []string{"Ana"}}.Apply(events)
func (c Criteria) Apply(events []event.Event) ([]event.Event, []string, []string) {
	directed := filterEvents(events, c.keepDirection)
	var people []personMatcher
	var matched, unknown []string
	for _, name := range c.People {
		person, found := resolvePerson(name, c.Direction, directed)
		if !found {
			unknown = append(unknown, name)
			continue
		}
		people, matched = append(people, person), append(matched, person.name)
	}
	if len(people) == 0 {
		return directed, nil, unknown
	}
	return filterEvents(directed, func(ev event.Event) bool { return matchesAnyPerson(ev, people) }), matched, unknown
}

// keepDirection applies the direction to Teams messages; other events
// always pass. "Received" excludes channel posts, which are published to a
// team rather than sent to the user.
func (c Criteria) keepDirection(ev event.Event) bool {
	if ev.Source != event.SourceTeams || c.Direction == AnyDirection {
		return true
	}
	sentByMe := ev.Metadata["sent_by_me"] == "true"
	if c.Direction == Sent {
		return sentByMe
	}
	return !sentByMe && ev.Metadata["conversation_kind"] != "canal"
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
