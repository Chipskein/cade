package rag

import (
	"context"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/timeline"
)

func teamsMessage(uid, sender string, hour int) event.Event {
	return event.Event{UID: uid, Source: event.SourceTeams, Timestamp: time.Date(2026, 9, 25, hour, 0, 0, 0, time.UTC),
		Content: sender + ": " + uid, Metadata: event.Metadata{"sender": sender, "sent_by_me": "false", "conversation_kind": "chat"}}
}

// storeOfMessages holds Marcos's and Ana's messages; "near" vectors point
// at the query direction {1, 0}.
func storeOfMessages() *testfakes.FakeEventStore {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{
		teamsMessage("ana-near", "Ana Goulart", 9), teamsMessage("marcos-far", "Marcos Lisboa", 10),
		teamsMessage("marcos-near", "Marcos Lisboa", 11), teamsMessage("marcos-old", "Marcos Lisboa", 8),
	}
	store.Events[3].Timestamp = store.Events[3].Timestamp.AddDate(0, 0, -30)
	store.Embeddings = map[string][]float32{
		"ana-near": {1, 0}, "marcos-far": {0, 1}, "marcos-near": {0.9, float32(math.Sqrt(1 - 0.81))}, "marcos-old": {1, 0},
	}
	return store
}

func restrictedAnswerer(store *testfakes.FakeEventStore, generator *testfakes.FakeGenerator) *Answerer {
	deps := Dependencies{Store: store, Embedder: &testfakes.FakeEmbedder{Vector: []float32{1, 0}}, Generator: generator,
		Now: func() time.Time { return fixedNow }, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	return NewAnswerer(deps, Settings{TopK: 5, MaxDistance: 0.5, MaxAnswerTokens: 100})
}

// Regression: "o que o Marcos me pediu ontem?" mixed in other people's
// messages, because names cannot be enforced by vector similarity.
func TestAnswerWithPersonUsesOnlyTheirEvents(t *testing.T) {
	generator := &testfakes.FakeGenerator{Reply: "O Marcos pediu [1]."}
	days, _ := timeline.ParseDayRange("2026-09-25", "", fixedNow)
	var matched []string
	observer := AnswerObserver{PeopleResolved: func(m, _ []string) { matched = m }}
	question := Question{Text: "o que o Marcos me pediu?", Days: &days, Criteria: listing.Criteria{People: []string{"Marcos"}}}
	answer, err := restrictedAnswerer(storeOfMessages(), generator).Answer(context.Background(), question, observer)
	if err != nil || len(answer.Evidence) != 2 || answer.Evidence[0].Event.UID != "marcos-near" || answer.Evidence[1].Event.UID != "marcos-far" {
		t.Fatalf("expected Marcos's two messages of the day, nearest first, got %+v (err %v)", answer.Evidence, err)
	}
	if len(matched) != 1 || matched[0] != "marcos" {
		t.Fatalf("expected marcos reported as matched, got %v", matched)
	}
}

func TestAnswerWithPersonHonoursTopK(t *testing.T) {
	answerer := restrictedAnswerer(storeOfMessages(), &testfakes.FakeGenerator{Reply: "x"})
	answerer.settings.TopK = 1
	question := Question{Text: "q", Criteria: listing.Criteria{People: []string{"Marcos"}}}
	answer, _ := answerer.Answer(context.Background(), question, AnswerObserver{})
	if len(answer.Evidence) != 1 {
		t.Fatalf("expected top-1, got %d", len(answer.Evidence))
	}
}

func TestFilterByTopicKeepsCloseEventsChronologically(t *testing.T) {
	store := storeOfMessages()
	kept, err := restrictedAnswerer(store, &testfakes.FakeGenerator{}).FilterByTopic(context.Background(), "redis", store.Events)
	if err != nil || len(kept) != 3 || kept[0].UID != "marcos-old" || kept[1].UID != "ana-near" || kept[2].UID != "marcos-near" {
		t.Fatalf("expected the three near events in time order, got %+v (err %v)", kept, err)
	}
}

func TestCosineDistance(t *testing.T) {
	if cosineDistance([]float32{1, 0}, []float32{1, 0}) != 0 || cosineDistance([]float32{1, 0}, []float32{0, 1}) != 1 {
		t.Fatal("expected 0 for identical and 1 for orthogonal unit vectors")
	}
}

func TestKeepSource(t *testing.T) {
	events := []event.Event{{Source: event.SourceGit}, {Source: event.SourceTeams}}
	if len(keepSource(events, event.SourceGit)) != 1 || len(keepSource(events, "")) != 2 {
		t.Fatal("unexpected source filtering")
	}
}

func TestIsScopedWithCriteria(t *testing.T) {
	if !(Question{Criteria: listing.Criteria{Direction: listing.Sent}}).IsScoped() {
		t.Fatal("criteria must count as scope")
	}
}
