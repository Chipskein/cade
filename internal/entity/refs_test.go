package entity

import (
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func TestRefsOfCommitNamesItsAuthor(t *testing.T) {
	ev := event.Event{Source: event.SourceGit, Metadata: event.Commit{Author: "Rui Costa", Email: "rui@example.com"}.Metadata()}
	want := []Ref{{Type: TypePerson, Key: "rui costa", Name: "rui costa", Role: RoleAuthor}}
	if got := RefsOf(ev); !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestRefsOfMessageMergesSpellingsAndSkipsTheConversation(t *testing.T) {
	ev := event.Event{Source: event.SourceTeams, Content: "Ana Souza: @Willian e @Wilian, revisem",
		Metadata: event.Message{Sender: "Ana Souza", Conversation: "Projeto X"}.Metadata()}
	want := []Ref{
		{Type: TypePerson, Key: "ana souza", Name: "ana souza", Role: RoleSender},
		{Type: TypePerson, Key: "wilian", Name: "willian", Role: RoleMentioned},
	}
	if got := RefsOf(ev); !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestRefsOfRepeatedMentionIsOneRef(t *testing.T) {
	ev := event.Event{Source: event.SourceTeams, Content: "@Ana veja; @Ana de novo", Metadata: event.Message{Sender: "Eu"}.Metadata()}
	mentions := 0
	for _, ref := range RefsOf(ev) {
		mentions += boolToCount(ref.Role == RoleMentioned)
	}
	if mentions != 1 {
		t.Fatalf("expected one mention ref, got %d in %+v", mentions, RefsOf(ev))
	}
}

func TestRefsOfFileEventIsItsFile(t *testing.T) {
	ev := event.Event{Source: event.SourceFile, Metadata: event.File{Path: "/home/eu/notas/plano.md"}.Metadata()}
	want := []Ref{{Type: TypeFile, Key: "/home/eu/notas/plano.md", Name: "plano.md", Role: RoleSubject}}
	if got := RefsOf(ev); !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestRefsOfVisitHasNoEntity(t *testing.T) {
	ev := event.Event{Source: event.SourceBrowser, Metadata: event.Visit{URL: "https://example.com", Title: "Exemplo"}.Metadata()}
	if got := RefsOf(ev); len(got) != 0 {
		t.Fatalf("expected no entities for a visit, got %+v", got)
	}
}

func boolToCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
