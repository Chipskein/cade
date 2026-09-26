package rag

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

func chatMessage(uid, sender, text string) event.Event {
	return event.Event{UID: uid, Source: event.SourceTeams, Timestamp: fixedNow, Content: sender + ": " + text + "\nConversa: chat X\nRecebida por você",
		Metadata: event.Metadata{"sender": sender}}
}

func TestIsChatter(t *testing.T) {
	chatter := []string{"ok", "valeu!", "bom dia", "Pode ser", "acho que sim", "blz, obrigado 👍", "kkkk", ""}
	for _, text := range chatter {
		if !isChatter(chatMessage("x", "Marcos Lima", text)) {
			t.Errorf("expected %q to be chatter", text)
		}
	}
	content := []string{"qual branch?", "subiu?", "PR aprovado", "reunião às 15h?", "ok, https://app.proj4.me/projects/14/tasks/162", "voltou, pode ignorar"}
	for _, text := range content {
		if isChatter(chatMessage("x", "Marcos Lima", text)) {
			t.Errorf("expected %q to be content", text)
		}
	}
}

func TestOnlyTeamsMessagesAreChatter(t *testing.T) {
	commit := event.Event{Source: event.SourceGit, Content: "ok"}
	if isChatter(commit) {
		t.Fatal("a commit is never chatter")
	}
}

// Regression: "ok"/"valeu" from Marcos outranked his actual request.
func TestRetrieveAmongDropsChatter(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	request, thanks := chatMessage("pedido", "Marcos Lima", "consegue revisar o PR do cache?"), chatMessage("valeu", "Marcos Lima", "valeu")
	store.Events = []event.Event{request, thanks}
	store.Embeddings = map[string][]float32{"pedido": {1, 0}, "valeu": {1, 0}}
	answerer, _ := newTestAnswerer(store, &testfakes.FakeGenerator{})
	hits, err := answerer.Retrieve(context.Background(), queryplan.Query{Question: "o que o Marcos me pediu?", Criteria: listing.Criteria{People: []string{"Marcos"}}}, AnswerObserver{})
	if err != nil || len(hits) != 1 || hits[0].Event.UID != "pedido" {
		t.Fatalf("expected only the request, got %+v (err %v)", hits, err)
	}
}

func TestSimilaritySearchDropsChatterAndKeepsTopK(t *testing.T) {
	var results []storage.ScoredEvent
	for _, uid := range []string{"a", "b", "c", "d", "e", "f"} {
		results = append(results, storage.ScoredEvent{Event: chatMessage(uid, "Ana", "ok"), Distance: 0.1})
	}
	results = append(results, scored("commit", event.SourceGit, 0.2))
	answerer, _ := newTestAnswerer(storeWithHits(results...), &testfakes.FakeGenerator{})
	hits, _ := answerer.Retrieve(context.Background(), queryplan.Query{Question: "x", Source: event.SourceGit}, AnswerObserver{})
	if len(hits) != 1 || hits[0].Event.UID != "commit" {
		t.Fatalf("expected chatter dropped, got %+v", hits)
	}
}
