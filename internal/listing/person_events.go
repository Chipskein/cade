package listing

import (
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/event"
)

// Erasing or exporting everything about one person (#86) starts from the
// people index: the person is a full name key, chosen among those the
// name the user typed matches, so "Ana" never takes Ana Prado and Ana
// Souza together. The store runs the same selection in SQL; this is its
// in-memory reference.

// PersonNames are the full name keys of the senders and authors a typed
// name matches as whole words, sorted: "ana" → ["ana prado", "ana souza"].
//
//	keys := listing.PersonNames(events, "Ana")
func PersonNames(events []event.Event, name string) []string {
	typed := " " + NameKey(name) + " "
	var keys []string
	for _, ev := range events {
		for _, entry := range authorEntries(ev) {
			if strings.Contains(" "+entry.Key+" ", typed) && !slices.Contains(keys, entry.Key) {
				keys = append(keys, entry.Key)
			}
		}
	}
	slices.Sort(keys)
	return keys
}

// EventsOfPerson keeps the events about the person with the full name key
// key: sent or written by them (an e-mail alias included), mentioning
// their first name, or in a one-to-one conversation with them, where
// every message the user did not send is theirs.
func EventsOfPerson(events []event.Event, key string) []event.Event {
	oneToOne := oneToOneConversations(events, key)
	return filterEvents(events, func(ev event.Event) bool {
		return wrote(ev, key) || mentions(ev, key) || oneToOne[ev.Message().ConversationID]
	})
}

func wrote(ev event.Event, key string) bool {
	return slices.ContainsFunc(authorEntries(ev), func(entry PersonEntry) bool { return entry.Key == key })
}

func mentions(ev event.Event, key string) bool {
	first, _, _ := strings.Cut(key, " ")
	return slices.ContainsFunc(PeopleOf(ev), func(entry PersonEntry) bool { return entry.Role == RoleMentioned && entry.Key == first })
}

// oneToOneConversations are the conversations whose every received
// message key sent.
func oneToOneConversations(events []event.Event, key string) map[string]bool {
	verdict := map[string]bool{}
	for _, ev := range events {
		message := ev.Message()
		if message.ConversationID == "" || message.SentByMe || message.Sender == "" {
			continue
		}
		previous, seen := verdict[message.ConversationID]
		verdict[message.ConversationID] = (previous || !seen) && wrote(ev, key)
	}
	return verdict
}

// authorEntries are the sender, author and author alias entries of ev.
func authorEntries(ev event.Event) []PersonEntry {
	return slices.DeleteFunc(PeopleOf(ev), func(entry PersonEntry) bool { return entry.Role != RoleSender && entry.Role != RoleAuthor })
}
