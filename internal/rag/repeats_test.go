package rag

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

func visitHit(uid, url string, when time.Time, distance float64) storage.ScoredEvent {
	return storage.ScoredEvent{Distance: distance, Event: event.Event{UID: uid, Source: event.SourceBrowser, Timestamp: when,
		Content: "Kubernetes probes", Metadata: event.Visit{URL: url, Title: "Kubernetes probes"}.Metadata()}}
}

func TestCollapseRepeatsKeepsClosestAndCountsOthers(t *testing.T) {
	early, late := fixedNow.Add(-48*time.Hour), fixedNow.Add(-time.Hour)
	hits := collapseRepeats([]storage.ScoredEvent{
		visitHit("a", "https://k8s.io/probes", early, 0.2),
		visitHit("b", "https://redis.io", early, 0.3),
		visitHit("c", "https://k8s.io/probes", late, 0.2),
		visitHit("d", "https://k8s.io/probes", early, 0.4),
	})
	if len(hits) != 2 || hits[0].Event.UID != "a" || hits[0].Repeats != 2 || !hits[0].LatestAt.Equal(late) || hits[1].Repeats != 0 {
		t.Fatalf("expected the probes page once with 2 repeats and the latest visit, got %+v", hits)
	}
}

// The same locator in another source is another thing.
func TestCollapseRepeatsSeparatesSources(t *testing.T) {
	page := visitHit("a", "/notas/a.md", fixedNow, 0.1)
	file := storage.ScoredEvent{Event: event.Event{UID: "f", Source: event.SourceFile, Metadata: event.File{Path: "/notas/a.md"}.Metadata()}}
	if hits := collapseRepeats([]storage.ScoredEvent{page, file}); len(hits) != 2 {
		t.Fatalf("expected both kept, got %d", len(hits))
	}
}

// Regression: 15 visits to one page took every top_k slot.
func TestRetrieveWidensSearchPastRepeats(t *testing.T) {
	var results []storage.ScoredEvent
	for i := range 40 {
		results = append(results, visitHit(fmt.Sprintf("v%d", i), "https://k8s.io/probes", fixedNow, 0.1))
	}
	for i := range 4 {
		results = append(results, visitHit(fmt.Sprintf("o%d", i), fmt.Sprintf("https://other.io/%d", i), fixedNow, 0.2))
	}
	answerer, _ := newTestAnswerer(storeWithHits(results...), &testfakes.FakeGenerator{})
	hits, err := answerer.Retrieve(context.Background(), queryplan.Query{Question: "probes", Source: event.SourceBrowser}, AnswerObserver{})
	if err != nil || len(hits) != 5 || hits[0].Repeats != 39 {
		t.Fatalf("expected the page once plus 4 others, got %d hits (err %v)", len(hits), err)
	}
}

func TestRepeatNote(t *testing.T) {
	hit := visitHit("a", "https://k8s.io", fixedNow, 0.1)
	if RepeatNote(hit, time.UTC) != "" {
		t.Fatal("expected no note without repeats")
	}
	hit.Repeats, hit.LatestAt = 11, time.Date(2026, 9, 25, 14, 10, 0, 0, time.UTC)
	if note := RepeatNote(hit, time.UTC); !strings.HasPrefix(note, "12 visitas, última em 2026-09-25") {
		t.Fatalf("unexpected note %q", note)
	}
	commit := storage.ScoredEvent{Event: event.Event{Source: event.SourceGit}, Repeats: 1, LatestAt: fixedNow}
	if !strings.HasPrefix(RepeatNote(commit, time.UTC), "2 ocorrências") {
		t.Fatalf("unexpected generic note %q", RepeatNote(commit, time.UTC))
	}
}

func TestPromptShowsRepeats(t *testing.T) {
	hit := visitHit("a", "https://k8s.io", fixedNow, 0.1)
	hit.Repeats, hit.LatestAt = 2, fixedNow
	if prompt := formatEvidence([]storage.ScoredEvent{hit}, time.UTC); !strings.Contains(prompt, "(3 visitas, última em") {
		t.Fatalf("expected the repeat note in the evidence, got %q", prompt)
	}
}

// A note deleted from its folder stays in the timeline, not in answers.
func TestRemovedFilesAreNotEvidence(t *testing.T) {
	removed := storage.ScoredEvent{Event: event.Event{UID: "gone", Source: event.SourceFile,
		Metadata: event.File{Path: "/notas/velha.md", RemovedAt: fixedNow}.Metadata()}}
	present := storage.ScoredEvent{Event: event.Event{UID: "here", Source: event.SourceFile, Metadata: event.File{Path: "/notas/nova.md"}.Metadata()}}
	if hits := withoutRemoved([]storage.ScoredEvent{removed, present}); len(hits) != 1 || hits[0].Event.UID != "here" {
		t.Fatalf("expected only the present file, got %+v", hits)
	}
}

func commitHit(uid string, authorship event.Authorship, distance float64) storage.ScoredEvent {
	return storage.ScoredEvent{Distance: distance, Event: event.Event{UID: uid, Source: event.SourceGit, Timestamp: fixedNow,
		Content: "commit " + uid, Metadata: event.Commit{Hash: uid, Authorship: authorship}.Metadata()}}
}

func TestFirstPersonEvidenceDropsOthersCommits(t *testing.T) {
	hits := []storage.ScoredEvent{commitHit("rui", event.AuthorshipOther, 0.1), commitHit("meu", event.AuthorshipMine, 0.2), commitHit("antigo", event.AuthorshipUnknown, 0.3)}
	own := usableEvidence(hits, queryplan.Query{OwnCommitsOnly: true})
	everyone := usableEvidence(hits, queryplan.Query{})
	if len(own) != 2 || own[0].Event.UID != "meu" || len(everyone) != 3 {
		t.Fatalf("expected the colleague's commit dropped only for first person, got %d and %d", len(own), len(everyone))
	}
}
