package entity

import (
	"path/filepath"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
)

// personRoles maps the people index roles that name a person. A
// conversation title is left out: a group's title ("Projeto X") is not a
// person, and 1:1 titles repeat the sender or the user.
var personRoles = map[listing.Role]Role{
	listing.RoleSender:    RoleSender,
	listing.RoleAuthor:    RoleAuthor,
	listing.RoleMentioned: RoleMentioned,
}

// RefsOf lists the entities ev refers to, once per role: the people the
// people index keeps (same names, same keys; the first spelling wins) and,
// for a file event, the file.
//
//	refs := entity.RefsOf(ev) // [{person "ana prado" "ana prado" sender}]
func RefsOf(ev event.Event) []Ref {
	refs := personRefs(ev)
	if file, found := fileRef(ev); found {
		refs = append(refs, file)
	}
	return refs
}

func personRefs(ev event.Event) []Ref {
	var refs []Ref
	seen := map[Ref]bool{}
	for _, entry := range listing.PeopleOf(ev) {
		role, isPerson := personRoles[entry.Role]
		link := Ref{Type: TypePerson, Key: entry.Key, Role: role}
		if !isPerson || seen[link] {
			continue
		}
		seen[link] = true
		refs = append(refs, Ref{Type: TypePerson, Key: entry.Key, Name: entry.Name, Role: role})
	}
	return refs
}

func fileRef(ev event.Event) (Ref, bool) {
	path := ev.File().Path
	if ev.Source != event.SourceFile || path == "" {
		return Ref{}, false
	}
	return Ref{Type: TypeFile, Key: path, Name: filepath.Base(path), Role: RoleSubject}, true
}
