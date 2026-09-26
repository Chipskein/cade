package teamssource

import (
	"strconv"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/v8value"
)

// Only user-authored messages are ingested. Other types carry XML metadata
// (calls, recordings, transcripts) or membership changes, not conversation.
var conversationMessageTypes = map[string]bool{"RichText/Html": true, "Text": true}

// System streams re-post messages from other conversations (the activity
// feed) or log calls; ingesting them duplicated announcements under an
// unknown sender.
var skippedThreadTypes = map[string]bool{"streamofnotifications": true, "streamofcalllogs": true}

// messageContext is what a message needs from the rest of the database.
type messageContext struct {
	conversations map[string]conversationInfo
	senders       map[string]string
	origin        string
}

// teamsMessage holds the fields read from one messageMap entry.
type teamsMessage struct {
	id             string
	conversationID string
	sender         string
	senderMRI      string
	text           string
	sentAt         time.Time
	sentByMe       bool
	// version changes on every edit (a millisecond stamp); it becomes the
	// event revision, so a re-ingest replaces the text with the newest.
	version string
}

// parseMessage extracts a conversation message, reporting false for
// system messages, deleted messages and messages without text.
func parseMessage(value *v8value.Value, senders map[string]string) (teamsMessage, bool) {
	if !isConversationMessage(value) {
		return teamsMessage{}, false
	}
	message := teamsMessage{
		id:             value.Get("id").String(),
		conversationID: value.Get("conversationId").String(),
		sender:         senderName(value, senders),
		senderMRI:      value.Get("creator").String(),
		text:           htmlToText(value.Get("content").String()),
		sentAt:         arrivalTime(value),
		sentByMe:       value.Get("isSentByCurrentUser").IsTrue(),
		version:        value.Get("version").String(),
	}
	valid := message.id != "" && message.conversationID != "" && message.text != "" && !message.sentAt.IsZero()
	return message, valid
}

func isConversationMessage(value *v8value.Value) bool {
	return conversationMessageTypes[value.Get("messageType").String()] &&
		!skippedThreadTypes[value.Get("threadType").String()] && !isDeleted(value)
}

// isDeleted reports a deletionInfo object; live messages have it undefined.
func isDeleted(value *v8value.Value) bool {
	info := value.Get("deletionInfo")
	return info != nil && info.Kind == v8value.KindObject
}

// senderName falls back to the profile cache by MRI when the message
// itself carries no display name.
func senderName(value *v8value.Value, senders map[string]string) string {
	for _, field := range []string{"imDisplayName", "fromDisplayNameInToken"} {
		if name := strings.TrimSpace(value.Get(field).String()); name != "" {
			return name
		}
	}
	return firstNonEmpty(senders[value.Get("creator").String()], "desconhecido")
}

// arrivalTime prefers the server timestamp; the client one is a fallback
// for messages still pending delivery.
func arrivalTime(value *v8value.Value) time.Time {
	for _, field := range []string{"originalArrivalTime", "clientArrivalTime"} {
		millis := value.Get(field)
		if millis != nil && millis.Kind == v8value.KindNumber && millis.Number > 0 {
			return time.UnixMilli(int64(millis.Number))
		}
	}
	return time.Time{}
}

// toEvent keys deduplication on conversation + message id: the same
// message cached by both Teams origins, or ingested twice, stays one event.
// Edits keep their id; the version is the revision that lets a re-ingest
// replace the stored text with the edited one.
func (m teamsMessage) toEvent(conversation conversationInfo, origin string) event.Event {
	return event.Event{
		UID:       event.StableID(event.SourceTeams, m.conversationID, m.id),
		Timestamp: m.sentAt,
		Source:    event.SourceTeams,
		Content:   m.sender + ": " + m.text + "\n" + conversationLine(conversation) + "\n" + directionLine(m.sentByMe, conversation.kind),
		Metadata: event.Metadata{
			"conversation_id": m.conversationID, "conversation": conversation.title, "conversation_kind": string(conversation.kind),
			"message_id": m.id, "sender": m.sender, "sender_mri": m.senderMRI,
			"sent_by_me": strconv.FormatBool(m.sentByMe), "origin": origin, event.RevisionKey: m.version,
		},
	}
}

// conversationLine and directionLine are part of the embedded text and of
// the model's evidence: without them "messages I received" matched team
// announcements, since nothing told a channel post from a direct message.
func conversationLine(conversation conversationInfo) string {
	if conversation.title == "" {
		return "Conversa: " + string(conversation.kind)
	}
	return "Conversa: " + string(conversation.kind) + " " + conversation.title
}

func directionLine(sentByMe bool, kind conversationKind) string {
	switch {
	case sentByMe:
		return "Enviada por você"
	case kind == kindChannel:
		return "Publicada no canal (não enviada diretamente a você)"
	}
	return "Recebida por você"
}
