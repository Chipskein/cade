package entity

import "github.com/chipskein/cade/internal/event"

// RelationsOf lists the relations ev states between the entities it
// refers to (RefsOf): today, a commit's author changed each of its files.
// Both ends are refs of the same event, so forgetting the event takes the
// relation and, with no other event, its entities.
//
//	relations := entity.RelationsOf(commit) // [{author changed file}]
func RelationsOf(ev event.Event) []RelationRef {
	refs := RefsOf(ev)
	author, found := refWithRole(refs, RoleAuthor)
	if !found {
		return nil
	}
	var relations []RelationRef
	for _, ref := range refs {
		if ref.Role == RoleChanged {
			relations = append(relations, RelationRef{From: author, Relation: RelationChanged, To: ref})
		}
	}
	return relations
}

func refWithRole(refs []Ref, role Role) (Ref, bool) {
	for _, ref := range refs {
		if ref.Role == role {
			return ref, true
		}
	}
	return Ref{}, false
}
