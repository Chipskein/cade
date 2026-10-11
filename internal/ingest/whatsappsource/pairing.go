package whatsappsource

import (
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/msgstore"
)

// Web message ids are "<from me>_<chat>_<key>[_<member>]".
const (
	webIDFromMePart  = 0
	webIDKeyPart     = 2
	webIDMinimumSize = 3
)

// messageKey identifies a message across clients: the Android key_id is
// the Web id's key part, and only the sender's own copy has from_me set.
type messageKey struct {
	fromMe bool
	keyID  string
}

// storedIndex finds the stored WhatsApp Web events by message key.
type storedIndex map[messageKey][]event.Event

func newStoredIndex(stored []event.Event) storedIndex {
	index := storedIndex{}
	for _, ev := range stored {
		if key, ok := webMessageKey(ev.Message().MessageID); ok {
			index[key] = append(index[key], ev)
		}
	}
	return index
}

func webMessageKey(messageID string) (messageKey, bool) {
	parts := strings.Split(messageID, webIDSeparator)
	if len(parts) < webIDMinimumSize {
		return messageKey{}, false
	}
	return messageKey{fromMe: parts[webIDFromMePart] == "true", keyID: parts[webIDKeyPart]}, true
}

// pair returns the stored event of the same message in the same chat.
func (index storedIndex) pair(message msgstore.Message) (event.Event, bool) {
	chats := candidateChats(message)
	for _, stored := range index[messageKey{fromMe: message.FromMe, keyID: message.KeyID}] {
		if chats[stored.Message().ConversationID] {
			return stored, true
		}
	}
	return event.Event{}, false
}
