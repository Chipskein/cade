package rag

import (
	"context"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

func TestIdentifierMatch(t *testing.T) {
	cases := map[string]string{
		"o que fiz no PROJ-481?":            `"proj 481"`,
		"erro ORA-01722":                    `"ora 01722"`,
		"o que foi o commit e5f6a7b?":       `e5f6a7b*`,
		"o que foi o PR 45?":                `"pull request 45" OR "merge request 45" OR "pr 45"`,
		"o que eu fiz relacionado a cache?": ``,
		"decade of facades":                 ``,
	}
	for question, expected := range cases {
		if got := identifierMatch(question); got != expected {
			t.Errorf("identifierMatch(%q) = %q, expected %q", question, got, expected)
		}
	}
}

func TestWordsMatchDropsQuestionWords(t *testing.T) {
	if got := wordsMatch("o que eu fiz sobre a migração do banco ontem?"); got != `"migracao" OR "banco"` {
		t.Fatalf("unexpected keyword query %q", got)
	}
}

func TestParseMode(t *testing.T) {
	if mode, err := ParseMode(""); err != nil || mode != ModeHybrid {
		t.Fatalf("expected hybrid by default, got %q (err %v)", mode, err)
	}
	if _, err := ParseMode("semantic"); err == nil || !strings.Contains(err.Error(), `"semantic"`) {
		t.Fatalf("expected an error naming the value, got %v", err)
	}
}

func TestFuseRankingsRewardsBothLists(t *testing.T) {
	a, b, c := scored("a", event.SourceGit, 0.1), scored("b", event.SourceGit, 0.2), scored("c", event.SourceGit, 0.3)
	fused := fuseRankings([]storage.ScoredEvent{a, b}, []storage.ScoredEvent{c, b})
	if len(fused) != 3 || fused[0].Event.UID != "b" {
		t.Fatalf("expected b, found by both, first; got %+v", fused)
	}
}

// Regression: a commit named by hash, or a PR by number, was never found
// by the vector search and the question was rejected.
func TestIdentifierQuestionUsesKeywordHits(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	commit := event.Event{UID: "g8", Source: event.SourceGit, Timestamp: fixedNow, Content: "Corrige conversão numérica"}
	store.Events = []event.Event{commit}
	store.Embeddings = map[string][]float32{"g8": {0, 1}}
	store.LexicalResults = []storage.ScoredEvent{{Event: commit, ChunkCount: 1}}
	answerer, _ := newTestAnswerer(store, &testfakes.FakeGenerator{})
	answerer.settings.MaxBestDistance = 0.1
	hits, err := answerer.Retrieve(context.Background(), queryplan.Query{Question: "o que foi o commit e5f6a7b?"}, AnswerObserver{})
	if err != nil || len(hits) != 1 || hits[0].Event.UID != "g8" || store.LexicalQueries[0].Match != "e5f6a7b*" {
		t.Fatalf("expected the commit found by hash despite the distance gate, got %+v (err %v)", hits, err)
	}
}

// Vector mode never runs keyword searches.
func TestVectorModeSkipsKeywords(t *testing.T) {
	store := storeWithHits(scored("a", event.SourceGit, 0.1))
	answerer, _ := newTestAnswerer(store, &testfakes.FakeGenerator{})
	answerer.settings.Mode = ModeVector
	answerer.Retrieve(context.Background(), queryplan.Query{Question: "o que foi o commit e5f6a7b?", Source: event.SourceGit}, AnswerObserver{})
	if len(store.LexicalQueries) != 0 {
		t.Fatalf("expected no keyword search, got %+v", store.LexicalQueries)
	}
}
