package provenance

import (
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func TestLocatorPerSource(t *testing.T) {
	cases := map[string]event.Event{
		"/src/api@79589eae1f":       {Source: event.SourceGit, Metadata: event.Metadata{"repository": "/src/api", "hash": "79589eae1f"}},
		"https://redis.io/docs":     {Source: event.SourceBrowser, Metadata: event.Metadata{"url": "https://redis.io/docs"}},
		"/home/eu/notas/reuniao.md": {Source: event.SourceFile, Metadata: event.Metadata{"path": "/home/eu/notas/reuniao.md"}},
		"cade:abc":                  {UID: "abc", Source: event.SourceGit},
		"https://teams.microsoft.com/l/message/19:abc@thread.v2/1727262000000": {Source: event.SourceTeams,
			Metadata: event.Metadata{"conversation_id": "19:abc@thread.v2", "message_id": "1727262000000"}},
	}
	for expected, ev := range cases {
		if got := Of(ev).Locator; got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	}
}

func TestTeamsLinkEscapesConversationID(t *testing.T) {
	link := teamsMessageLink("19:a b/c@thread.v2", "1")
	if link != "https://teams.microsoft.com/l/message/19:a%20b%2Fc@thread.v2/1" {
		t.Fatalf("unexpected link %q", link)
	}
}

func TestAllKeepsOrderAndFields(t *testing.T) {
	when := time.Date(2026, 9, 25, 9, 30, 0, 0, time.UTC)
	refs := All([]event.Event{{UID: "a", Source: event.SourceGit, Timestamp: when, Content: "Corrige login\n\ncorpo"}, {UID: "b"}})
	if len(refs) != 2 || refs[0].UID != "a" || refs[0].Summary != "Corrige login" || !refs[0].OccurredAt.Equal(when) || refs[1].UID != "b" {
		t.Fatalf("unexpected references %+v", refs)
	}
}
