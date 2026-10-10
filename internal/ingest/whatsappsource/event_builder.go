package whatsappsource

import (
	"maps"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/msgstore"
)

// attachmentKey holds a media message's file name, so a photo without
// caption still says what it was.
const attachmentKey = "attachment"

// eventBuilder turns database messages into events, pairing each with
// the stored Web event of the same message when there is one.
type eventBuilder struct {
	origin string
	index  storedIndex
	names  contactNames
}

func newEventBuilder(origin string, stored []event.Event) eventBuilder {
	return eventBuilder{origin: origin, index: newStoredIndex(stored), names: newContactNames(stored)}
}

// build returns the message's event and whether it completes a stored one.
func (b eventBuilder) build(message msgstore.Message) (event.Event, bool) {
	if stored, found := b.index.pair(message); found {
		return completed(stored, message), true
	}
	return b.newEvent(message), false
}

// completed is the stored event with the database's text: same UID, time,
// names and schema marks, so it replaces the stored one instead of adding
// a second.
func completed(stored event.Event, message msgstore.Message) event.Event {
	if message.Text == "" {
		return stored
	}
	text := stored.Message()
	text.Text = message.Text
	metadata := maps.Clone(stored.Metadata)
	maps.Copy(metadata, backupMetadata(text, message))
	stored.Content, stored.Metadata = text.Content(), metadata
	return stored
}

// newEvent keys the message as the WhatsApp schema would, so a later
// `ingest whatsapp` reading it from the Web finds it stored.
func (b eventBuilder) newEvent(message msgstore.Message) event.Event {
	conversationID, messageID := chatID(message), webMessageID(message)
	text := event.Message{
		ConversationID: conversationID, Conversation: b.names.conversation(message), Kind: event.KindOther,
		MessageID: messageID, Sender: b.names.sender(message), SenderMRI: senderID(message),
		SentByMe: message.FromMe, Origin: b.origin, Text: message.Text,
	}
	return event.Event{
		UID:       event.StableID(event.SourceWhatsApp, conversationID, messageID),
		Timestamp: message.SentAt,
		Source:    event.SourceWhatsApp,
		Content:   text.Content(),
		Metadata:  backupMetadata(text, message),
	}
}

// backupMetadata is text's metadata marked as read from the backup, with
// the attachment's name.
func backupMetadata(text event.Message, message msgstore.Message) event.Metadata {
	metadata := text.Metadata()
	if text.Text != "" {
		metadata[event.TextOriginKey] = event.TextOriginBackup
	}
	if message.MediaName != "" {
		metadata[attachmentKey] = message.MediaName
	}
	return metadata
}
