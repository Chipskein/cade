package listing

import (
	"regexp"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/textnorm"
)

// mentionPattern captures the first word after "@": the Teams ingestor
// renders mentions as "@Display Name".
var mentionPattern = regexp.MustCompile(`@(\p{L}+)`)

// addressees tells a group message addressed to someone else ("pronto?
// @Vitor") from one addressed to the user. Regression: such messages were
// listed as "received" for "o que a Ana me passou ontem". People are known
// by the first word of their name, as mentions are usually written.
type addressees struct {
	self   map[string]bool
	people map[string]bool
}

// addresseesOf learns the user's name from the messages they sent and
// everyone else's from the senders in events.
func addresseesOf(events []event.Event) addressees {
	known := addressees{self: map[string]bool{}, people: map[string]bool{}}
	for _, ev := range events {
		first := firstNameWord(ev.Metadata["sender"])
		if ev.Source != event.SourceTeams || first == "" {
			continue
		}
		known.people[first] = true
		if ev.Metadata["sent_by_me"] == "true" {
			known.self[first] = true
		}
	}
	return known
}

// addressedToOthers is true only when every mention is a known person and
// none is the user; a tag or team mention ("@INTERNO"), or not knowing the
// user's name, keeps the message as received.
func (a addressees) addressedToOthers(ev event.Event) bool {
	mentioned := mentionedFirstNames(ev.Content)
	if len(a.self) == 0 || len(mentioned) == 0 {
		return false
	}
	for _, name := range mentioned {
		if a.self[name] || !a.people[name] {
			return false
		}
	}
	return true
}

func mentionedFirstNames(text string) []string {
	var names []string
	for _, match := range mentionPattern.FindAllStringSubmatch(text, -1) {
		names = append(names, textnorm.Fold(match[1]))
	}
	return names
}

func firstNameWord(name string) string {
	words := nameWords(textnorm.Fold(name))
	if len(words) == 0 {
		return ""
	}
	return words[0]
}
