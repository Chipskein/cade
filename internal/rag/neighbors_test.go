package rag

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

// neighborStore holds a commit found by the search and, in its store, the
// commit itself plus pages read at several distances from it.
func neighborStore() *testfakes.FakeEventStore {
	commit := scored("commit", event.SourceGit, 0.2)
	store := storeWithHits(commit)
	at := commit.Event.Timestamp
	page := func(uid string, offset time.Duration) event.Event {
		return event.Event{UID: uid, Source: event.SourceBrowser, Timestamp: at.Add(offset), Content: "page " + uid,
			Metadata: event.Visit{URL: "https://example.com/" + uid}.Metadata()}
	}
	store.Events = []event.Event{page("p3h", -3*time.Hour), page("p90", -90*time.Minute), page("p15", -15*time.Minute),
		commit.Event, page("p5", 5*time.Minute), page("p30", 30*time.Minute), page("p60", time.Hour), page("p100", 100*time.Minute)}
	return store
}

func evidenceUIDs(hits []storage.ScoredEvent) []string {
	uids := make([]string, len(hits))
	for i, hit := range hits {
		uids[i] = hit.Event.UID
	}
	return uids
}

func TestTimeQuestionBringsTheClosestNeighborsInTime(t *testing.T) {
	answerer, _ := newTestAnswerer(neighborStore(), &testfakes.FakeGenerator{})
	hits, err := answerer.Retrieve(context.Background(), queryplan.Query{Question: "o que eu lia Quando fiz o commit?"}, AnswerObserver{})
	want := []string{"commit", "p5", "p15", "p30", "p60"}
	if got := evidenceUIDs(hits); err != nil || !slices.Equal(got, want) {
		t.Fatalf("expected %q, got %q (err %v)", want, got, err)
	}
	if hits[1].Distance != hits[0].Distance {
		t.Fatalf("expected a neighbor to carry its anchor's distance, got %.2f and %.2f", hits[0].Distance, hits[1].Distance)
	}
}

func TestQuestionWithoutTimeCueGetsNoNeighbors(t *testing.T) {
	answerer, _ := newTestAnswerer(neighborStore(), &testfakes.FakeGenerator{})
	hits, err := answerer.Retrieve(context.Background(), queryplan.Query{Question: "o que fiz no commit?"}, AnswerObserver{})
	if got := evidenceUIDs(hits); err != nil || len(got) != 1 {
		t.Fatalf("expected only the commit, got %q (err %v)", got, err)
	}
}

func TestTimeNeighborsKeepTheSourceFilter(t *testing.T) {
	answerer, _ := newTestAnswerer(neighborStore(), &testfakes.FakeGenerator{})
	query := queryplan.Query{Question: "commits before the release", Source: event.SourceGit}
	hits, err := answerer.Retrieve(context.Background(), query, AnswerObserver{})
	if got := evidenceUIDs(hits); err != nil || len(got) != 1 {
		t.Fatalf("expected no browser neighbor for a git question, got %q (err %v)", got, err)
	}
}

func TestAsksForTimeContextMatchesWholeWords(t *testing.T) {
	for question, want := range map[string]bool{"o que vi Enquanto revisava?": true, "what did I read before lunch?": true, "quandoo": false, "o que fiz ontem?": false,
		"quando é o deploy?": false, "When is the deploy?": false, "o que eu lia quando corrigi?": true} {
		if got := asksForTimeContext(question); got != want {
			t.Errorf("asksForTimeContext(%q) = %v, want %v", question, got, want)
		}
	}
}
