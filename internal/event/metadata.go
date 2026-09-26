package event

import (
	"strconv"
	"strings"
	"time"
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
	Authorship Authorship
}

// Authorship says whether a commit is the user's. Commits are kept either
// way ("o que o Rui commitou?"), but "o que eu fiz?" must not count a
// colleague's work.
type Authorship string

const (
	// AuthorshipUnknown: stored before identities were known; treated as
	// the user's, so nothing disappears until the next git ingestion marks
	// it.
	AuthorshipUnknown Authorship = ""
	AuthorshipMine    Authorship = "mine"
	AuthorshipOther   Authorship = "other"
)

// IsOthersCommit reports a commit known to be someone else's.
func (e Event) IsOthersCommit() bool {
	return e.Source == SourceGit && e.Commit().Authorship == AuthorshipOther
}

// Visit is a browser event's metadata.
type Visit struct {
	Browser string
	URL     string
	Title   string
	// History is the history file the visit was read from.
	History string
}

// File is a file event's metadata. One event per path holds the current
// version: ModifiedAt is its revision, so a newer version replaces it.
// RemovedAt is set when the file disappeared from its directory.
type File struct {
	Path       string
	Size       int64
	ModifiedAt time.Time
	RemovedAt  time.Time
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
	// Text is the message as written, without the lines Content adds;
	// empty for events ingested before it was kept (schema version 1).
	Text string
}

// Metadata keys, as stored.
const (
	keyRepository     = "repository"
	keyHash           = "hash"
	keyAuthor         = "author"
	keyEmail          = "email"
	keyFiles          = "files"
	keyAuthorship     = "authorship"
	keyBrowser        = "browser"
	keyURL            = "url"
	keyTitle          = "title"
	keyHistory        = "history"
	keyPath           = "path"
	keySize           = "size"
	keyRemovedAt      = "removed_at"
	keyConversationID = "conversation_id"
	keyConversation   = "conversation"
	keyKind           = "conversation_kind"
	keyMessageID      = "message_id"
	keySender         = "sender"
	keySenderMRI      = "sender_mri"
	keySentByMe       = "sent_by_me"
	keyOrigin         = "origin"
	keyText           = "text"
)

// Metadata is the stored form of c.
func (c Commit) Metadata() Metadata {
	return Metadata{keyRepository: c.Repository, keyHash: c.Hash, keyAuthor: c.Author, keyEmail: c.Email, keyFiles: strings.Join(c.Files, "\n"),
		keyAuthorship: string(c.Authorship)}
}

// Commit reads e's metadata as a commit; fields absent from it are empty.
func (e Event) Commit() Commit {
	m := e.Metadata
	return Commit{Repository: m[keyRepository], Hash: m[keyHash], Author: m[keyAuthor], Email: m[keyEmail], Files: splitLines(m[keyFiles]),
		Authorship: Authorship(m[keyAuthorship])}
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
	return Metadata{keyPath: f.Path, keySize: strconv.FormatInt(f.Size, 10),
		RevisionKey: nanosText(f.ModifiedAt), keyRemovedAt: nanosText(f.RemovedAt)}
}

// File reads e's metadata as a file.
func (e Event) File() File {
	size, _ := strconv.ParseInt(e.Metadata[keySize], 10, 64)
	return File{Path: e.Metadata[keyPath], Size: size, ModifiedAt: nanosTime(e.Metadata[RevisionKey]), RemovedAt: nanosTime(e.Metadata[keyRemovedAt])}
}

// RemovedAtKey is the metadata entry the store sets when a file vanished.
const RemovedAtKey = keyRemovedAt

// Times are stored as Unix nanoseconds, which also order as revisions;
// the zero time is stored as "".
func nanosText(moment time.Time) string {
	if moment.IsZero() {
		return ""
	}
	return strconv.FormatInt(moment.UnixNano(), 10)
}

func nanosTime(text string) time.Time {
	nanos, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, nanos)
}

// Metadata is the stored form of m.
func (m Message) Metadata() Metadata {
	return Metadata{keyConversationID: m.ConversationID, keyConversation: m.Conversation, keyKind: string(m.Kind),
		keyMessageID: m.MessageID, keySender: m.Sender, keySenderMRI: m.SenderMRI, keySentByMe: strconv.FormatBool(m.SentByMe),
		keyOrigin: m.Origin, RevisionKey: m.Revision, keyText: m.Text}
}

// Message reads e's metadata as a Teams message.
func (e Event) Message() Message {
	m := e.Metadata
	return Message{ConversationID: m[keyConversationID], Conversation: m[keyConversation], Kind: ConversationKind(m[keyKind]),
		MessageID: m[keyMessageID], Sender: m[keySender], SenderMRI: m[keySenderMRI], SentByMe: m[keySentByMe] == "true",
		Origin: m[keyOrigin], Revision: m[RevisionKey], Text: m[keyText]}
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
