// Package export writes what cade keeps as JSON Lines, one event per line
// with what was derived from it, so the user can take a complete,
// machine-readable copy (#85; PRIVACY, access and portability). Vectors
// are left out: they mean nothing outside the model that made them.
package export

import (
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

// Record is one line of the export.
type Record struct {
	UID             string           `json:"uid"`
	Source          event.Source     `json:"source"`
	OccurredAt      time.Time        `json:"occurred_at"`
	Content         string           `json:"content"`
	Metadata        event.Metadata   `json:"metadata"`
	People          []Person         `json:"people,omitempty"`
	Entities        []Entity         `json:"entities,omitempty"`
	Relations       []Relation       `json:"relations,omitempty"`
	Representations []Representation `json:"representations,omitempty"`
}

// Person is a name the people index keeps for the event.
type Person struct {
	Role listing.Role `json:"role"`
	Name string       `json:"name"`
}

// EntityKey names an entity across the export.
type EntityKey struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// Entity is what the event refers to, and how.
type Entity struct {
	EntityKey
	Name string `json:"name"`
	Role string `json:"role"`
}

// Relation is what the event states between two entities.
type Relation struct {
	From       EntityKey `json:"from"`
	Relation   string    `json:"relation"`
	To         EntityKey `json:"to"`
	Confidence *float64  `json:"confidence,omitempty"`
}

// Representation is what a model derived from one of the event's
// entities, such as an image's description.
type Representation struct {
	Entity   EntityKey `json:"entity"`
	Modality string    `json:"modality"`
	Kind     string    `json:"kind"`
	Model    string    `json:"model"`
	Revision int       `json:"revision"`
	Content  string    `json:"content"`
}

func keyOf(stored storage.Entity) EntityKey {
	return EntityKey{Type: string(stored.Type), Key: stored.Key}
}

func peopleOf(ev event.Event) []Person {
	var people []Person
	for _, entry := range listing.PeopleOf(ev) {
		people = append(people, Person{Role: entry.Role, Name: entry.Name})
	}
	return people
}

func relationsOf(stored []storage.Relationship) []Relation {
	var relations []Relation
	for _, relation := range stored {
		relations = append(relations, Relation{From: keyOf(relation.From), Relation: string(relation.Relation), To: keyOf(relation.To), Confidence: relation.Confidence})
	}
	return relations
}

func representationsOf(owner storage.Entity, stored []storage.StoredRepresentation) []Representation {
	var representations []Representation
	for _, rep := range stored {
		representations = append(representations, Representation{Entity: keyOf(owner), Modality: string(rep.Modality), Kind: string(rep.Kind),
			Model: rep.Model, Revision: rep.Revision, Content: rep.Content})
	}
	return representations
}
