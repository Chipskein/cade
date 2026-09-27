package listing

import (
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/textnorm"
)

// The store keeps who appears in each event and each message's direction
// (the event_people table and events.direction), so a person question
// without a period is filtered in SQL instead of loading the whole
// history. This file defines what goes in that index; Matches and
// Direction.keep apply the same rules in memory.

// Role says where a name appears in an event.
type Role string

const (
	RoleSender       Role = "sender"
	RoleAuthor       Role = "author"
	RoleConversation Role = "conversation"
	// RoleMentioned is a first name after "@" in a Teams message.
	RoleMentioned Role = "mentioned"
)

// PersonEntry is one name of an event as the index stores it. Name is
// folded words ("ana prado"); Key merges spelling variants (NameKey).
type PersonEntry struct {
	Role Role
	Name string
	Key  string
}

// PeopleOf lists the names the index keeps for ev: the sender (or, for a
// commit, the author), the conversation and, in a Teams message, each
// mentioned first name. Names without letters are left out: they match
// nobody.
//
//	entries := listing.PeopleOf(ev) // [{sender "ana prado" "ana prado"} ...]
func PeopleOf(ev event.Event) []PersonEntry {
	message := ev.Message()
	var entries []PersonEntry
	entries = appendName(entries, senderRole(message), firstNonEmpty(message.Sender, ev.Commit().Author))
	entries = appendName(entries, RoleConversation, message.Conversation)
	if ev.Source != event.SourceTeams {
		return entries
	}
	for _, name := range mentionedFirstNames(ev.Content) {
		entries = append(entries, PersonEntry{Role: RoleMentioned, Name: name, Key: spellingKeys(name)})
	}
	return entries
}

func senderRole(message event.Message) Role {
	if message.Sender != "" {
		return RoleSender
	}
	return RoleAuthor
}

func appendName(entries []PersonEntry, role Role, text string) []PersonEntry {
	words := nameWords(textnorm.Fold(text))
	if len(words) == 0 {
		return entries
	}
	name := strings.Join(words, " ")
	return append(entries, PersonEntry{Role: role, Name: name, Key: spellingKeys(name)})
}

// Directions as the index stores them. A channel post is published to a
// team, so neither sent to nor received by the user.
const (
	indexedSent     = "sent"
	indexedReceived = "received"
	indexedChannel  = "channel"
)

// IndexedDirection is ev's direction as the index stores it: "sent",
// "received" or "channel" for a Teams message, "" for any other event,
// which every direction keeps.
func IndexedDirection(ev event.Event) string {
	if ev.Source != event.SourceTeams {
		return ""
	}
	message := ev.Message()
	switch {
	case message.SentByMe:
		return indexedSent
	case message.Kind == event.KindChannel:
		return indexedChannel
	}
	return indexedReceived
}

// Indexed is the stored direction d keeps, besides events with none; ""
// for AnyDirection, which keeps everything.
func (d Direction) Indexed() string {
	switch d {
	case Sent:
		return indexedSent
	case Received:
		return indexedReceived
	}
	return ""
}
