package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testfakes"
)

func rochaCommit(uid string) event.Event {
	return event.Event{UID: uid, Source: event.SourceGit, Timestamp: time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC), Content: "commit " + uid,
		Metadata: event.Commit{Repository: "/src/api", Hash: uid, Author: "F. Rocha", Email: "fernanda.rocha@example.com"}.Metadata()}
}

func runAuthored(t *testing.T, store *testfakes.FakeEventStore, aliases bool, commits ...event.Event) {
	t.Helper()
	pipeline := newTestPipeline(store, &testfakes.FakeEmbedder{}).WithEmailAliases(aliases)
	if _, err := pipeline.Run(context.Background(), AuthoredFake{FakeCollector{Events: commits}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEmailAliasesAreOffByDefault(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	runAuthored(t, store, false, rochaCommit("c1"))
	if alias := store.Events[0].Commit().AuthorAlias; alias != "" {
		t.Fatalf("expected no alias without the option, got %q", alias)
	}
}

func TestEmailAliasesFillNewAndStoredCommits(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	runAuthored(t, store, false, rochaCommit("old"))
	runAuthored(t, store, true, rochaCommit("new"))
	for _, stored := range store.Events {
		if alias := stored.Commit().AuthorAlias; alias != "fernanda rocha" {
			t.Errorf("expected %s aliased as fernanda rocha, got %q", stored.UID, alias)
		}
	}
}

// Rule 1: turning the option off deletes what it kept.
func TestTurningEmailAliasesOffDeletesThem(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	runAuthored(t, store, true, rochaCommit("c1"))
	runAuthored(t, store, false)
	if alias := store.Events[0].Commit().AuthorAlias; alias != "" {
		t.Fatalf("expected the alias deleted, got %q", alias)
	}
}

func TestNoAliasWhenTheEmailNamesTheAuthor(t *testing.T) {
	pipeline := newTestPipeline(testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}).WithEmailAliases(true)
	commit := event.Event{Source: event.SourceGit, Metadata: event.Commit{Author: "Rui Costa", Email: "rui.costa@example.com"}.Metadata()}
	if alias := pipeline.aliased(commit).Commit().AuthorAlias; alias != "" {
		t.Fatalf("expected no alias repeating the author, got %q", alias)
	}
}
