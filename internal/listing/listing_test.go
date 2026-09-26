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

// Regression: "mensagens que enviei pro avilla" found nobody, as the chat
// is with "Leandro Avila".
func TestContainsNameToleratesDoubledLettersAndY(t *testing.T) {
	if !containsName("Leandro Avila - Oficina5", "avilla") || !containsName("Willian Gabriel", "wilian") || !containsName("Thaysa Lima", "thaisa") {
		t.Fatal("expected spelling variants to match")
	}
	if containsName("Bruna Souza", "bruno") || containsName("Paula", "paulo") {
		t.Fatal("expected different names to stay different")
	}
}

func TestNameKeyMergesSpellingVariants(t *testing.T) {
	if NameKey("Willian") != NameKey("wilian") || NameKey("Bruno") == NameKey("Bruna") {
		t.Fatal("unexpected name keys")
	}
}

func groupMessage(sender, text string, sentByMe bool) event.Event {
	mine := "false"
	if sentByMe {
		mine = "true"
	}
	return event.Event{UID: sender + text, Source: event.SourceTeams, Content: sender + ": " + text,
		Metadata: event.Metadata{"sender": sender, "sent_by_me": mine, "conversation": "INTERNO", "conversation_kind": "chat"}}
}

// Regression: Ana's group messages mentioning Marcos or Vitor were listed
// as received by the user.
func TestReceivedExcludesMessagesMentioningOnlyOthers(t *testing.T) {
	events := []event.Event{
		groupMessage("Bruno Nascimento - Oficina5", "bom dia", true),
		groupMessage("Marcos Lisboa - Oficina5", "ok", false),
		groupMessage("Vitor Hugo - Oficina5", "ok", false),
		groupMessage("Ana Goulart - Oficina5", "@Marcos Lisboa - Oficina5 consegue ver?", false),
		groupMessage("Ana Goulart - Oficina5", "pronto? @Vitor Hugo - Oficina5", false),
		groupMessage("Ana Goulart - Oficina5", "@Bruno Nascimento - Oficina5 e @Marcos Lisboa olhem isso", false),
		groupMessage("Ana Goulart - Oficina5", "@INTERNO O5 deploy às 19h", false),
		groupMessage("Ana Goulart - Oficina5", "sem menção nenhuma", false),
	}
	kept, _, _ := Criteria{Direction: Received, People: []string{"Ana"}}.Apply(events)
	var texts []string
	for _, ev := range kept {
		texts = append(texts, ev.Content)
	}
	joined := strings.Join(texts, "|")
	if len(kept) != 3 || strings.Contains(joined, "consegue ver") || strings.Contains(joined, "pronto?") {
		t.Fatalf("expected the messages to others dropped, got %q", texts)
	}
}

func TestMentionsKeptWhenUserUnknown(t *testing.T) {
	events := []event.Event{groupMessage("Marcos Lima", "ok", false), groupMessage("Ana Souza", "@Marcos pode ver?", false)}
	if kept, _, _ := (Criteria{Direction: Received}).Apply(events); len(kept) != 2 {
		t.Fatalf("expected no mention filtering without the user's name, got %d", len(kept))
	}
}

func TestMentionedFirstNamesIgnoresEmailDomainsAsPeople(t *testing.T) {
	known := addresseesOf([]event.Event{groupMessage("Eu Mesmo", "oi", true), groupMessage("Rui Costa", "oi", false)})
	if known.addressedToOthers(groupMessage("Rui Costa", "manda para eu@example.com", false)) {
		t.Fatal("an e-mail domain is not a person; the message stays received")
	}
	if !known.addressedToOthers(groupMessage("Ana", "@Rui olha isso", false)) {
		t.Fatal("expected a mention of Rui only to be addressed to others")
	}
}
