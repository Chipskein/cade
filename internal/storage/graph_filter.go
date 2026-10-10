package storage

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/textnorm"
)

// GraphFilter narrows events by what they hold and who or what appears in
// them (#24). Its zero value keeps every event. The store runs it in SQL;
// Keeps is its in-memory reference.
type GraphFilter struct {
	// Modality keeps events that hold that kind of entity: an image file,
	// or a message with an image attached.
	Modality entity.Type
	// Appearing keeps events whose text has every word of each name:
	// the sender of a message, a person cited in an image's description,
	// a project on a slide.
	Appearing []string
	// EntityIDs keeps events linked to any of these entities, reached by
	// widening through the relationships.
	EntityIDs []int64
}

// IsEmpty reports a filter that keeps everything.
func (g GraphFilter) IsEmpty() bool {
	return g.Modality == "" && len(g.Appearing) == 0 && len(g.EntityIDs) == 0
}

// minAppearingWordLength drops connectors ("do", "da") from a name: the
// model writes "projeto do Orion" as often as "projeto Orion".
const minAppearingWordLength = 3

var nonWordRunes = regexp.MustCompile(`[^a-z0-9]+`)

// AppearingWords are the folded words of name that must appear, as the
// text index tokenizes them.
//
//	storage.AppearingWords("projeto do Órion") // ["projeto", "orion"]
func AppearingWords(name string) []string {
	var words []string
	for _, word := range strings.Fields(nonWordRunes.ReplaceAllString(textnorm.Fold(name), " ")) {
		if utf8.RuneCountInString(word) >= minAppearingWordLength {
			words = append(words, word)
		}
	}
	return words
}

// Keeps reports whether ev passes the modality and appearing names; links
// lists the entity ids ev refers to, for EntityIDs.
func (g GraphFilter) Keeps(ev event.Event, links []int64) bool {
	if g.Modality != "" && !Holds(ev, g.Modality) {
		return false
	}
	if len(g.EntityIDs) > 0 && !slices.ContainsFunc(links, func(id int64) bool { return slices.Contains(g.EntityIDs, id) }) {
		return false
	}
	return appearsAll(ev, g.Appearing)
}

// Holds reports whether ev holds an entity of kind: an image file shows an
// Image; a message holds one when its attachment is an image.
func Holds(ev event.Event, kind entity.Type) bool {
	if slices.ContainsFunc(entity.RefsOf(ev), func(ref entity.Ref) bool { return ref.Type == kind && ref.Role == entity.RoleShows }) {
		return true
	}
	return kind == entity.TypeImage && imagefile.IsDescribedImage(ev.Attachment())
}

// appearsAll looks in the whole content; the store looks chunk by chunk,
// which differs only for words split across a long text's chunks. A name
// without words to look for keeps nothing.
func appearsAll(ev event.Event, names []string) bool {
	text := " " + strings.Join(AppearingWords(ev.Content), " ") + " "
	for _, name := range names {
		if !hasEveryWord(text, AppearingWords(name)) {
			return false
		}
	}
	return true
}

// hasEveryWord reports whether padded (words between spaces) has every
// one of words, and at least one.
func hasEveryWord(padded string, words []string) bool {
	for _, word := range words {
		if !strings.Contains(padded, " "+word+" ") {
			return false
		}
	}
	return len(words) > 0
}
