package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func sentMessage(when time.Time, conversation, text string) event.Event {
	return event.Event{UID: conversation + text + when.String(), Source: event.SourceTeams, Timestamp: when, Content: "Eu: " + text,
		Metadata: event.Metadata{"sent_by_me": "true", "sender": "Eu", "conversation": conversation}}
}

func sentToSeveralPeople(world *fakeWorld) {
	yesterday := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	world.store.Events = []event.Event{
		sentMessage(yesterday, "Eu, Leandro Avila", "voltou, pode ignorar"),
		sentMessage(yesterday.Add(time.Hour), "Eu, Carla Dias", "bom dia"),
		sentMessage(yesterday.Add(2*time.Hour), "Eu, Carla Dias", "o pedido da Zenite subiu"),
	}
}

// Regression: a misspelled name matched nobody, so every message sent that
// day was listed.
func TestAskListSentToMisspelledPerson(t *testing.T) {
	world := newFakeWorld()
	sentToSeveralPeople(world)
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Avilla"], "direcao": "enviadas", "assunto": null, "status": null}`
	_, stdout, _ := world.run("ask", "quais foram as mensagens que enviei pro avilla ontem")
	if !strings.Contains(stdout, "1 eventos") || !strings.Contains(stdout, "voltou, pode ignorar") {
		t.Fatalf("expected only the message to Avila, got:\n%s", stdout)
	}
}

func TestAskListUnknownNameFiltersAsText(t *testing.T) {
	world := newFakeWorld()
	sentToSeveralPeople(world)
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Zenite"], "direcao": "enviadas", "assunto": null, "status": null}`
	_, stdout, stderr := world.run("ask", "mensagens que enviei sobre a Zenite ontem")
	if !strings.Contains(stderr, "buscando como texto: Zenite") || !strings.Contains(stdout, "1 eventos") || !strings.Contains(stdout, "pedido da Zenite") {
		t.Fatalf("expected only the message mentioning Zenite, got %q:\n%s", stderr, stdout)
	}
}

func TestEventsMentioningAllWithoutTermsKeepsAll(t *testing.T) {
	events := []event.Event{{Content: "a"}, {Content: "b"}}
	if kept := eventsMentioningAll(events, nil); len(kept) != 2 {
		t.Fatalf("expected every event, got %d", len(kept))
	}
}
