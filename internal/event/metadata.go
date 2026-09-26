package event

import (
	"strconv"
	"strings"
)

// Each source's metadata has a typed view. The stored form stays the flat
// string map (a JSON object in the database) with the same keys, so the
// types add compile-time names and parsing in one place without migrating
// stored events.

// Commit is a git event's metadata.
type Commit struct {
	Repository string
	Hash       string
	Author     string
	Email      string
	Files      []string
}

// Visit is a browser event's metadata.
type Visit struct {
	Browser string
	URL     string
	Title   string
	// History is the history file the visit was read from.
	History string
}

// File is a file event's metadata.
type File struct {
	Path string
	Size int64
}

// ConversationKind says who a Teams message was addressed to.
type ConversationKind string

const (
	KindChat    ConversationKind = "chat"
	KindChannel ConversationKind = "canal"
	KindMeeting ConversationKind = "reunião"
	KindOther   ConversationKind = "conversa"
)

// Message is a Teams event's metadata.
type Message struct {
	ConversationID string
	Conversation   string
	Kind           ConversationKind
	MessageID      string
	Sender         string
	SenderMRI      string
	SentByMe       bool
	// Origin is the IndexedDB the message was read from.
	Origin   string
	Revision string
}

// Metadata keys, as stored.
const (
	keyRepository     = "repository"
	keyHash           = "hash"
	keyAuthor         = "author"
	keyEmail          = "email"
	keyFiles          = "files"
	keyBrowser        = "browser"
	keyURL            = "url"
	keyTitle          = "title"
	keyHistory        = "history"
	keyPath           = "path"
	keySize           = "size"
	keyConversationID = "conversation_id"
	keyConversation   = "conversation"
	keyKind           = "conversation_kind"
	keyMessageID      = "message_id"
	keySender         = "sender"
	keySenderMRI      = "sender_mri"
	keySentByMe       = "sent_by_me"
	keyOrigin         = "origin"
)

// Metadata is the stored form of c.
func (c Commit) Metadata() Metadata {
	return Metadata{keyRepository: c.Repository, keyHash: c.Hash, keyAuthor: c.Author, keyEmail: c.Email, keyFiles: strings.Join(c.Files, "\n")}
}

// Commit reads e's metadata as a commit; fields absent from it are empty.
func (e Event) Commit() Commit {
	m := e.Metadata
	return Commit{Repository: m[keyRepository], Hash: m[keyHash], Author: m[keyAuthor], Email: m[keyEmail], Files: splitLines(m[keyFiles])}
}

// Metadata is the stored form of v.
func (v Visit) Metadata() Metadata {
	return Metadata{keyBrowser: v.Browser, keyURL: v.URL, keyTitle: v.Title, keyHistory: v.History}
}

// Visit reads e's metadata as a page visit.
func (e Event) Visit() Visit {
	m := e.Metadata
	return Visit{Browser: m[keyBrowser], URL: m[keyURL], Title: m[keyTitle], History: m[keyHistory]}
}

// Metadata is the stored form of f.
func (f File) Metadata() Metadata {
	return Metadata{keyPath: f.Path, keySize: strconv.FormatInt(f.Size, 10)}
}

// File reads e's metadata as a file.
func (e Event) File() File {
	size, _ := strconv.ParseInt(e.Metadata[keySize], 10, 64)
	return File{Path: e.Metadata[keyPath], Size: size}
}

// Metadata is the stored form of m.
func (m Message) Metadata() Metadata {
	return Metadata{keyConversationID: m.ConversationID, keyConversation: m.Conversation, keyKind: string(m.Kind),
		keyMessageID: m.MessageID, keySender: m.Sender, keySenderMRI: m.SenderMRI, keySentByMe: strconv.FormatBool(m.SentByMe),
		keyOrigin: m.Origin, RevisionKey: m.Revision}
}

// Message reads e's metadata as a Teams message.
func (e Event) Message() Message {
	m := e.Metadata
	return Message{ConversationID: m[keyConversationID], Conversation: m[keyConversation], Kind: ConversationKind(m[keyKind]),
		MessageID: m[keyMessageID], Sender: m[keySender], SenderMRI: m[keySenderMRI], SentByMe: m[keySentByMe] == "true",
		Origin: m[keyOrigin], Revision: m[RevisionKey]}
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
