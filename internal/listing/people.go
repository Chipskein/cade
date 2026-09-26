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

// resolvePerson tries the full name, then its first word ("Ana Goulart"
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
	sender := firstNonEmpty(ev.Metadata["sender"], ev.Metadata["author"])
	return p.asSender && containsName(sender, p.name) ||
		p.inConversation && containsName(ev.Metadata["conversation"], p.name)
}

func matchesAnyPerson(ev event.Event, people []personMatcher) bool {
	for _, person := range people {
		if person.matches(ev) {
			return true
		}
	}
	return false
}

// containsName matches whole words, so "ana" finds "Ana Goulart" but not
// "Ianne" or "Mariana".
func containsName(text, name string) bool {
	joined := " " + strings.Join(nameWords(textnorm.Fold(text)), " ") + " "
	return strings.Contains(joined, " "+name+" ")
}

func nameWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) })
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
