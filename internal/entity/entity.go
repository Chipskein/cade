// Package entity names the things that exist across events (a person, a
// file) as opposed to the events where they appear. The store derives an
// event's entities from the event itself (RefsOf), so ingestion and the
// schema migration link the same entities and collectors stay unaware.
package entity

// Type is what an entity is, stored as entities.type. The types share one
// table; what differs between them lives in the entity's metadata.
type Type string

const (
	TypePerson     Type = "person"
	TypeFile       Type = "file"
	TypeImage      Type = "image"
	TypeRepository Type = "repository"
	TypeProject    Type = "project"
	TypeMessage    Type = "message"
	TypeURL        Type = "url"
	TypeObject     Type = "object"
)

// Role is how an event refers to an entity, stored as event_entities.role.
type Role string

const (
	RoleSender    Role = "sender"
	RoleAuthor    Role = "author"
	RoleMentioned Role = "mentioned"
	// RoleSubject is the entity the event is about: a file's own event.
	RoleSubject Role = "subject"
	// RoleChanged is a file a commit changed.
	RoleChanged Role = "changed"
)

// Relation is how one entity relates to another, stored as
// relationships.relation; each relation comes from one event.
type Relation string

const (
	// RelationChanged: a commit's author changed one of its files.
	RelationChanged Relation = "changed"
)

// Ref is one entity an event refers to. Key identifies the entity within
// its type (a person's listing.NameKey, a file's path); Name is how this
// event writes it, kept as the entity's name when it is first seen.
type Ref struct {
	Type Type
	Key  string
	Name string
	Role Role
}

// RelationRef is one relation an event states between two of its refs.
type RelationRef struct {
	From     Ref
	Relation Relation
	To       Ref
}
