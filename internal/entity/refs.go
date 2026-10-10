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
// people index keeps (same names, same keys; the first spelling wins), for
// a file event its file and the image it holds, and for a commit the files
// it changed.
//
//	refs := entity.RefsOf(ev) // [{person "ana prado" "ana prado" sender}]
func RefsOf(ev event.Event) []Ref {
	refs := append(personRefs(ev), changedFileRefs(ev)...)
	if file, found := fileRef(ev); found {
		refs = append(refs, file)
	}
	if image, found := imageRef(ev); found {
		refs = append(refs, image)
	}
	return refs
}

// imageRef is the image an event holds (an image file, a picture in a
// Teams message), keyed by its SHA-256 so a moved, renamed, copied or
// re-sent image is the same entity; named after the first file seen with
// it, or the first address.
func imageRef(ev event.Event) (Ref, bool) {
	hash := ev.Image().SHA256
	if hash == "" {
		return Ref{}, false
	}
	return Ref{Type: TypeImage, Key: hash, Name: imageName(ev), Role: RoleShows}, true
}

func imageName(ev event.Event) string {
	if urls := ev.ImageURLs(); ev.Source != event.SourceFile && len(urls) > 0 {
		return urls[0]
	}
	return filepath.Base(ev.File().Path)
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

// changedFileRefs are a commit's files as File entities. The key is the
// path under the repository, so a file the files source also indexes is
// the same entity in both.
func changedFileRefs(ev event.Event) []Ref {
	commit := ev.Commit()
	if ev.Source != event.SourceGit || commit.Repository == "" {
		return nil
	}
	var refs []Ref
	seen := map[string]bool{}
	for _, file := range commit.Files {
		path := filepath.Join(commit.Repository, file)
		if file == "" || seen[path] {
			continue
		}
		seen[path] = true
		refs = append(refs, Ref{Type: TypeFile, Key: path, Name: filepath.Base(path), Role: RoleChanged})
	}
	return refs
}
