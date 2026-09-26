package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testfakes"
)

// FakeCollector emits a fixed list of events.
type FakeCollector struct {
	Events []event.Event
}

func (f FakeCollector) CollectEvents(_ context.Context, emit EmitFunc) error {
	for _, ev := range f.Events {
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}

func newTestPipeline(store *testfakes.FakeEventStore, embedder *testfakes.FakeEmbedder) *Pipeline {
	return NewPipeline(store, embedder, "doc: ", slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func twoEvents() FakeCollector {
	return FakeCollector{Events: []event.Event{
		{UID: "a", Source: event.SourceGit, Timestamp: time.Unix(1, 0), Content: "fix bug"},
		{UID: "b", Source: event.SourceFile, Timestamp: time.Unix(2, 0)},
	}}
}

func TestRunStoresEventsAndReports(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	report, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(), nil)
	if err != nil || report != (Report{Collected: 2, Inserted: 2}) || len(store.Events) != 2 {
		t.Fatalf("expected 2 inserted, got %+v with %d stored (err %v)", report, len(store.Events), err)
	}
}

func TestRunTwiceDoesNotDuplicate(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	pipeline := newTestPipeline(store, &testfakes.FakeEmbedder{})
	pipeline.Run(context.Background(), twoEvents(), nil)
	report, _ := pipeline.Run(context.Background(), twoEvents(), nil)
	if report != (Report{Collected: 2, AlreadyStored: 2}) || len(store.Events) != 2 {
		t.Fatalf("expected re-run to skip both, got %+v with %d stored", report, len(store.Events))
	}
}

func TestRunSkipsEmbeddingForKnownEvents(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	pipeline := newTestPipeline(store, embedder)
	pipeline.Run(context.Background(), twoEvents(), nil)
	pipeline.Run(context.Background(), twoEvents(), nil)
	if len(embedder.Inputs) != 1 {
		t.Fatalf("expected a single embedding call overall, got %d", len(embedder.Inputs))
	}
}

func TestRunEmbedsWithDocumentPrefix(t *testing.T) {
	embedder := &testfakes.FakeEmbedder{}
	newTestPipeline(testfakes.NewFakeEventStore(), embedder).Run(context.Background(), twoEvents(), nil)
	if embedder.Inputs[0] != "doc: fix bug" {
		t.Fatalf("expected prefixed text, got %q", embedder.Inputs[0])
	}
}

func TestRunStoresEventWithoutContentWithoutEmbedding(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(), nil)
	if store.Embeddings["b"] != nil || store.Embeddings["a"] == nil {
		t.Fatalf("expected embedding only for content-bearing event, got %v", store.Embeddings)
	}
}

func TestRunAbortsOnEmbeddingFailure(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	embedder := &testfakes.FakeEmbedder{FailWith: errors.New("model crashed")}
	_, err := newTestPipeline(store, embedder).Run(context.Background(), twoEvents(), nil)
	if err == nil || len(store.Events) != 0 {
		t.Fatalf("expected abort with nothing stored, got err %v and %d stored", err, len(store.Events))
	}
}

func TestRunReportsProgressAfterEachEvent(t *testing.T) {
	var seen []Report
	newTestPipeline(testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(),
		func(report Report) { seen = append(seen, report) })
	if len(seen) != 2 || seen[0] != (Report{Collected: 1, Inserted: 1}) || seen[1].Collected != 2 {
		t.Fatalf("expected running totals after each event, got %+v", seen)
	}
}

func TestTruncateRunesKeepsWholeCharacters(t *testing.T) {
	if got := truncateRunes("ãéí", 2); got != "ãé" {
		t.Fatalf("expected %q, got %q", "ãé", got)
	}
}

func TestTruncateRunesShortTextUnchanged(t *testing.T) {
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Fatalf("expected unchanged text, got %q", got)
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 || boolToInt(false) != 0 {
		t.Fatal("expected true=1 false=0")
	}
}

func TestFindSource(t *testing.T) {
	specs := []SourceSpec{{Name: "git"}, {Name: "files"}}
	found, err := FindSource(specs, "files")
	_, missingErr := FindSource(specs, "teams")
	if err != nil || found.Name != "files" || missingErr == nil || !strings.Contains(missingErr.Error(), "teams") {
		t.Fatalf("expected files found and teams rejected, got %+v, %v, %v", found, err, missingErr)
	}
}

func TestSourceNames(t *testing.T) {
	names := SourceNames([]SourceSpec{{Name: "git"}, {Name: "browser"}})
	if strings.Join(names, ",") != "git,browser" {
		t.Fatalf("expected [git browser], got %v", names)
	}
}

func teamsVersion(content, revision string) event.Event {
	return event.Event{UID: "m", Source: event.SourceTeams, Timestamp: time.Unix(1, 0), Content: content,
		Metadata: event.Metadata{event.RevisionKey: revision}}
}

// Regression: an edited Teams message kept its first ingested text forever.
func TestRunReplacesEditedEventAndReembeds(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	pipeline := newTestPipeline(store, embedder)
	pipeline.Run(context.Background(), FakeCollector{Events: []event.Event{teamsVersion("deploy às 18h", "100")}}, nil)
	report, err := pipeline.Run(context.Background(), FakeCollector{Events: []event.Event{teamsVersion("deploy às 19h", "200")}}, nil)
	if err != nil || report != (Report{Collected: 1, Updated: 1}) || store.Events[0].Content != "deploy às 19h" || len(embedder.Inputs) != 2 {
		t.Fatalf("expected the edit stored and re-embedded, got %+v, %q, %d embeds (err %v)", report, store.Events[0].Content, len(embedder.Inputs), err)
	}
}

// Two Teams caches can hold different versions; the newest must win
// whatever the order, and nothing may alternate on the next run.
func TestRunKeepsNewestRevision(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	pipeline := newTestPipeline(store, &testfakes.FakeEmbedder{})
	versions := FakeCollector{Events: []event.Event{teamsVersion("nova", "200"), teamsVersion("antiga", "100")}}
	pipeline.Run(context.Background(), versions, nil)
	report, _ := pipeline.Run(context.Background(), versions, nil)
	if store.Events[0].Content != "nova" || report.Updated != 0 || len(store.Updated) != 0 {
		t.Fatalf("expected the newest kept without updates, got %q, %+v, updates %v", store.Events[0].Content, report, store.Updated)
	}
}

func TestReplaces(t *testing.T) {
	cases := []struct {
		incoming, stored event.Event
		expected         bool
	}{
		{teamsVersion("a", "1"), teamsVersion("a", "1"), false},
		{teamsVersion("b", "2"), teamsVersion("a", "1"), true},
		{teamsVersion("b", "1"), teamsVersion("a", "2"), false},
		{teamsVersion("b", "1"), teamsVersion("a", "1"), false},
		{teamsVersion("b", "2"), teamsVersion("a", ""), true},
		{event.Event{Content: "novo título"}, event.Event{Content: "título"}, true},
	}
	for i, c := range cases {
		if got := replaces(c.incoming, c.stored); got != c.expected {
			t.Errorf("case %d: expected %v, got %v", i, c.expected, got)
		}
	}
}
