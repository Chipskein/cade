package listing

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/textnorm"
)

// PersonMatcher is a resolved name and where to look for it.
type PersonMatcher struct {
	// Name is folded ("ana prado"), as it matched.
	Name string
	// AsSender matches the message sender or commit author; InConversation
	// matches the conversation name, which lists the participants of 1:1
	// chats.
	AsSender       bool
	InConversation bool
}

// matcherFor derives where a person must appear from the direction:
// messages received from X are sent by X; messages sent to X are the
// user's own, in a conversation with X.
func matcherFor(name string, direction Direction) PersonMatcher {
	switch direction {
	case Received:
		return PersonMatcher{Name: name, AsSender: true}
	case Sent:
		return PersonMatcher{Name: name, InConversation: true}
	}
	return PersonMatcher{Name: name, AsSender: true, InConversation: true}
}

// resolvePerson tries the full name, then its first word ("Ana Prado"
// may be written "Ana"), keeping the first that exists reports as
// matching some event.
func resolvePerson(name string, direction Direction, exists PersonProbe) (PersonMatcher, bool, error) {
	for _, candidate := range nameCandidates(textnorm.Fold(name)) {
		person := matcherFor(candidate, direction)
		found, err := exists(person)
		if err != nil || found {
			return person, found, err
		}
	}
	return PersonMatcher{}, false, nil
}

func nameCandidates(name string) []string {
	words := nameWords(name)
	if len(words) < 2 {
		return words
	}
	return []string{strings.Join(words, " "), words[0]}
}

// Matches reports whether ev has the person where p looks; a commit's
// author alias counts as its author.
func (p PersonMatcher) Matches(ev event.Event) bool {
	sender := firstNonEmpty(ev.Message().Sender, ev.Commit().Author)
	asSender := containsName(sender, p.Name) || containsName(ev.Commit().AuthorAlias, p.Name)
	return p.AsSender && asSender || p.InConversation && containsName(ev.Message().Conversation, p.Name)
}

// Key is the name as the people index stores it (see NameKey).
func (p PersonMatcher) Key() string {
	return spellingKeys(p.Name)
}

// Roles are the index roles where p looks: the index has one entry per
// role, and a match on any of them is a match.
func (p PersonMatcher) Roles() []Role {
	var roles []Role
	if p.AsSender {
		roles = append(roles, RoleSender, RoleAuthor)
	}
	if p.InConversation {
		roles = append(roles, RoleConversation)
	}
	return roles
}

func matchesAnyPerson(ev event.Event, people []PersonMatcher) bool {
	for _, person := range people {
		if person.Matches(ev) {
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

// A name read from an e-mail needs a first and a last name of two letters
// or more, so a handle ("abfn0905", "eu") does not pass as one.
const (
	minEmailNameWords      = 2
	minEmailNameWordLength = 2
)

// NameFromEmail reads a name from an e-mail's local part, folded, or ""
// when it does not look like one.
//
//	listing.NameFromEmail("fernanda.rocha@example.com") // "fernanda rocha"
func NameFromEmail(email string) string {
	local, _, found := strings.Cut(email, "@")
	if !found {
		return ""
	}
	words := slices.DeleteFunc(nameWords(textnorm.Fold(local)), func(word string) bool {
		return utf8.RuneCountInString(word) < minEmailNameWordLength
	})
	if len(words) < minEmailNameWords {
		return ""
	}
	return strings.Join(words, " ")
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
