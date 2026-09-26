package listing

import (
	"strings"
	"unicode"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/textnorm"
)

// personMatcher is a resolved name and where to look for it.
type personMatcher struct {
	name string
	// asSender matches the message author; inConversation matches the
	// conversation name, which lists the participants of 1:1 chats.
	asSender       bool
	inConversation bool
}

// matcherFor derives where a person must appear from the direction:
// messages received from X are sent by X; messages sent to X are the
// user's own, in a conversation with X.
func matcherFor(name string, direction Direction) personMatcher {
	switch direction {
	case Received:
		return personMatcher{name: name, asSender: true}
	case Sent:
		return personMatcher{name: name, inConversation: true}
	}
	return personMatcher{name: name, asSender: true, inConversation: true}
}

// resolvePerson tries the full name, then its first word ("Ana Prado"
// may be written "Ana"), keeping the first that matches some event.
func resolvePerson(name string, direction Direction, events []event.Event) (personMatcher, bool) {
	for _, candidate := range nameCandidates(textnorm.Fold(name)) {
		person := matcherFor(candidate, direction)
		for _, ev := range events {
			if person.matches(ev) {
				return person, true
			}
		}
	}
	return personMatcher{}, false
}

func nameCandidates(name string) []string {
	words := nameWords(name)
	if len(words) < 2 {
		return words
	}
	return []string{strings.Join(words, " "), words[0]}
}

func (p personMatcher) matches(ev event.Event) bool {
	sender := firstNonEmpty(ev.Message().Sender, ev.Commit().Author)
	return p.asSender && containsName(sender, p.name) ||
		p.inConversation && containsName(ev.Message().Conversation, p.name)
}

func matchesAnyPerson(ev event.Event, people []personMatcher) bool {
	for _, person := range people {
		if person.matches(ev) {
			return true
		}
	}
	return false
}

// containsName matches whole words, so "ana" finds "Ana Prado" but not
// "Ianne" or "Mariana", and tolerates spelling variants (see spellingKey).
func containsName(text, name string) bool {
	joined := " " + spellingKeys(textnorm.Fold(text)) + " "
	return strings.Contains(joined, " "+spellingKeys(name)+" ")
}

func nameWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) })
}

// NameKey is how a name is compared: folded, whole words, spelling
// variants merged, so "Wilian" and "willian" have the same key.
//
//	listing.NameKey("Willian") == listing.NameKey("wilian") // true
func NameKey(name string) string {
	return spellingKeys(textnorm.Fold(name))
}

func spellingKeys(text string) string {
	words := nameWords(text)
	for i, word := range words {
		words[i] = spellingKey(word)
	}
	return strings.Join(words, " ")
}

// spellingKey ignores doubled letters and y/i, how names are commonly
// misspelled: "sillva" finds "Silva", "wilian" finds "Willian". An edit
// distance would also merge different people (Bruno, Bruna).
func spellingKey(word string) string {
	var key []rune
	for _, letter := range strings.ReplaceAll(word, "y", "i") {
		if len(key) == 0 || key[len(key)-1] != letter {
			key = append(key, letter)
		}
	}
	return string(key)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
