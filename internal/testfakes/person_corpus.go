package testfakes

import (
	"time"

	"github.com/chipskein/cade/internal/event"
)

// PersonCorpus mixes what erasing Ana Prado must tell apart: her one-to-one
// chat (both ways), a group where she writes and where someone mentions
// her, a commit known by her e-mail alone, and another Ana.
func PersonCorpus() []event.Event {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	chat := func(uid, sender, conversation, text string, sentByMe bool) event.Event {
		message := event.Message{ConversationID: conversation, Conversation: conversation, Kind: event.KindChat, Sender: sender, SentByMe: sentByMe, Text: text}
		return event.Event{UID: uid, Source: event.SourceTeams, Timestamp: at, Content: sender + ": " + text, Metadata: message.Metadata()}
	}
	commit := func(uid, author, alias string) event.Event {
		return event.Event{UID: uid, Source: event.SourceGit, Timestamp: at, Content: "commit " + uid,
			Metadata: event.Commit{Author: author, AuthorAlias: alias, Hash: uid}.Metadata()}
	}
	return []event.Event{
		chat("ana-1to1", "Ana Prado", "c1", "oi, tudo bem?", false), chat("me-1to1", "Eu", "c1", "tudo e você?", true),
		chat("ana-group", "Ana Prado", "c2", "subi o deploy", false), chat("caio-group", "Caio Mendes", "c2", "valeu", false),
		chat("caio-mention", "Caio Mendes", "c2", "@Ana olha isso", false), chat("me-group", "Eu", "c2", "beleza", true),
		commit("alias", "A. Prado", "ana prado"), commit("souza", "Ana Souza", ""),
	}
}
