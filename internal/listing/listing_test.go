package listing

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func message(sender, conversation, sentByMe, kind string) event.Event {
	return event.Event{Source: event.SourceTeams, Content: sender, Metadata: event.Metadata{
		"sender": sender, "conversation": conversation, "sent_by_me": sentByMe, "conversation_kind": kind,
	}}
}

var dayOfMessages = []event.Event{
	message("Ana Goulart - Oficina5", "Ana Goulart - Oficina5, Bruno Nascimento", "false", "chat"),
	message("Ianne Melo - Oficina5", "INTERNO O5 - TMT", "false", "chat"),
	message("Mariana Souza", "INTERNO O5 - MGM", "false", "chat"),
	message("Bruno Nascimento", "Ana Goulart - Oficina5, Bruno Nascimento", "true", "chat"),
	message("Ana Goulart - Oficina5", "Oficina5 › Avisos", "false", "canal"),
}

func senders(events []event.Event) string {
	var names []string
	for _, ev := range events {
		names = append(names, ev.Metadata["sender"]+"@"+ev.Metadata["conversation_kind"])
	}
	return strings.Join(names, "|")
}

func TestApplyReceivedFromPerson(t *testing.T) {
	kept, matched, _ := Criteria{Direction: Received, People: []string{"Ana"}}.Apply(dayOfMessages)
	if senders(kept) != "Ana Goulart - Oficina5@chat" || matched[0] != "ana" {
		t.Fatalf("expected Ana's direct message only (not Ianne, Mariana or the channel post), got %s", senders(kept))
	}
}

func TestApplyWithPersonIncludesBothSides(t *testing.T) {
	kept, _, _ := Criteria{People: []string{"ana"}}.Apply(dayOfMessages)
	if senders(kept) != "Ana Goulart - Oficina5@chat|Bruno Nascimento@chat|Ana Goulart - Oficina5@canal" {
		t.Fatalf("expected Ana's messages and the user's replies to her, got %s", senders(kept))
	}
}

func TestApplySentToPerson(t *testing.T) {
	kept, _, _ := Criteria{Direction: Sent, People: []string{"Ana Goulart"}}.Apply(dayOfMessages)
	if senders(kept) != "Bruno Nascimento@chat" {
		t.Fatalf("expected the user's message in the chat with Ana, got %s", senders(kept))
	}
}

func TestApplyReportsUnknownPeople(t *testing.T) {
	kept, matched, unknown := Criteria{People: []string{"Bigfertil"}}.Apply(dayOfMessages)
	if len(kept) != len(dayOfMessages) || matched != nil || unknown[0] != "Bigfertil" {
		t.Fatalf("expected no filtering and Bigfertil reported, got %d %v %v", len(kept), matched, unknown)
	}
}

func TestApplyDirectionOnlyPassesOtherSources(t *testing.T) {
	events := append([]event.Event{{Source: event.SourceGit}}, dayOfMessages...)
	kept, _, _ := Criteria{Direction: Sent}.Apply(events)
	if len(kept) != 2 || kept[0].Source != event.SourceGit {
		t.Fatalf("expected the commit and the user's message, got %+v", kept)
	}
}

func TestIsEmpty(t *testing.T) {
	if !(Criteria{}).IsEmpty() || (Criteria{People: []string{"a"}}).IsEmpty() || (Criteria{Direction: Sent}).IsEmpty() {
		t.Fatal("unexpected IsEmpty")
	}
}

func TestContainsNameWholeWords(t *testing.T) {
	if !containsName("Ana Goulart - Oficina5", "ana") || containsName("Mariana", "ana") || containsName("Ianne", "ana") {
		t.Fatal("expected whole-word matching")
	}
}

func TestNameCandidates(t *testing.T) {
	if got := nameCandidates("ana goulart"); len(got) != 2 || got[0] != "ana goulart" || got[1] != "ana" {
		t.Fatalf("expected [ana goulart ana], got %v", got)
	}
}

func TestMatcherUsesGitAuthor(t *testing.T) {
	commit := event.Event{Source: event.SourceGit, Metadata: event.Metadata{"author": "Ana Goulart"}}
	if !matcherFor("ana", AnyDirection).matches(commit) {
		t.Fatal("expected git author to count as sender")
	}
}
