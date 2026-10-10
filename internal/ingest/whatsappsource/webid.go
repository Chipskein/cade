package whatsappsource

import (
	"strconv"
	"strings"

	"github.com/chipskein/cade/internal/msgstore"
)

// Android and WhatsApp Web spell the same JIDs differently; the Web's
// spelling is the one stored, because the schema keys its events on it.
const (
	androidUserServer = "@s.whatsapp.net"
	webUserServer     = "@c.us"
	groupServer       = "@g.us"
	webIDSeparator    = "_"
)

// webJID spells an Android JID as WhatsApp Web does.
func webJID(jid string) string {
	return strings.Replace(jid, androidUserServer, webUserServer, 1)
}

// chatID is the conversation id WhatsApp Web gives the chat: its LID when
// the database maps one, as the Web did for 4,118 of 4,120 one-to-one
// messages measured in #77.
func chatID(message msgstore.Message) string {
	if message.ChatLID != "" {
		return message.ChatLID
	}
	return webJID(message.ChatJID)
}

// senderID is who wrote a received message: the group member, or the
// chat itself in a one-to-one conversation, as the schema's sender_id.
func senderID(message msgstore.Message) string {
	switch {
	case message.SenderLID != "":
		return message.SenderLID
	case message.SenderJID != "":
		return webJID(message.SenderJID)
	}
	return chatID(message)
}

func isGroup(message msgstore.Message) bool {
	return strings.HasSuffix(message.ChatJID, groupServer)
}

// webMessageID rebuilds WhatsApp Web's message id, "<from me>_<chat>_<key>"
// plus "_<member>" for a group message received from someone. It matched
// the Web's own for 95% of the messages both had in #77, so a message
// first read from the backup is usually not duplicated by the Web later.
//
//	webMessageID(msgstore.Message{KeyID: "AAAA", ChatJID: "55…@s.whatsapp.net"}) // "false_55…@c.us_AAAA"
func webMessageID(message msgstore.Message) string {
	parts := []string{strconv.FormatBool(message.FromMe), chatID(message), message.KeyID}
	if isGroup(message) && !message.FromMe && message.SenderJID != "" {
		parts = append(parts, senderID(message))
	}
	return strings.Join(parts, webIDSeparator)
}

// candidateChats are the conversation ids a stored Web message of this
// chat may have: the Web used either the phone JID or the LID.
func candidateChats(message msgstore.Message) map[string]bool {
	return map[string]bool{webJID(message.ChatJID): true, chatID(message): true}
}
