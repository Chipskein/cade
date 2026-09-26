// Package provenance ties results back to the records they came from: every
// answer, listing or task can name the exact commit, page, file or message
// behind it, so the user can check it in the original tool.
package provenance

import (
	"net/url"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// Reference identifies an event precisely enough to find the original.
type Reference struct {
	UID        string       `json:"uid"`
	Source     event.Source `json:"source"`
	OccurredAt time.Time    `json:"occurred_at"`
	// Locator is where the original lives: "repo@hash" for a commit, the
	// URL of a page, the path of a file, a Teams link for a message.
	Locator string `json:"locator"`
	Summary string `json:"summary"`
}

// Of returns ev's reference; sources without a known locator fall back to
// the event UID, which `cade` itself can always resolve.
//
//	ref := provenance.Of(commitEvent) // ref.Locator == "/src/api@79589eae…"
func Of(ev event.Event) Reference {
	return Reference{UID: ev.UID, Source: ev.Source, OccurredAt: ev.Timestamp, Locator: locator(ev), Summary: ev.Headline()}
}

func locator(ev event.Event) string {
	switch {
	case ev.Source == event.SourceGit && ev.Commit().Hash != "":
		return ev.Commit().Repository + "@" + ev.Commit().Hash
	case ev.Source == event.SourceBrowser && ev.Visit().URL != "":
		return ev.Visit().URL
	case ev.Source == event.SourceFile && ev.File().Path != "":
		return ev.File().Path
	case ev.Source == event.SourceTeams && ev.Message().MessageID != "":
		return teamsMessageLink(ev.Message().ConversationID, ev.Message().MessageID)
	}
	return "cade:" + ev.UID
}

// teamsMessageLink is Teams' deep link to a message, which opens it in the
// client; conversation ids ("19:abc@thread.v2") need escaping.
func teamsMessageLink(conversationID, messageID string) string {
	return "https://teams.microsoft.com/l/message/" + url.PathEscape(conversationID) + "/" + url.PathEscape(messageID)
}

// All returns the references of events, in order.
func All(events []event.Event) []Reference {
	references := make([]Reference, len(events))
	for i, ev := range events {
		references[i] = Of(ev)
	}
	return references
}
