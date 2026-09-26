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
		{teamsVersion("a", "2"), teamsVersion("a", "1"), true},
	}
	for i, c := range cases {
		if got := replaces(c.incoming, c.stored); got != c.expected {
			t.Errorf("case %d: expected %v, got %v", i, c.expected, got)
		}
	}
}

func visitAt(uid string, second int64) event.Event {
	return event.Event{UID: uid, Source: event.SourceBrowser, Timestamp: time.Unix(second, 0), Content: "Kubernetes probes\nhttps://k8s.io/probes"}
}

// Regression: every revisit of a page was embedded again, though its text
// (title and URL) is identical.
func TestRunReusesVectorOfIdenticalText(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	visits := FakeCollector{Events: []event.Event{visitAt("v1", 1), visitAt("v2", 2), visitAt("v3", 3)}}
	report, err := newTestPipeline(store, embedder).Run(context.Background(), visits, nil)
	if err != nil || report.Inserted != 3 || len(embedder.Inputs) != 1 || store.Embeddings["v3"] == nil {
		t.Fatalf("expected 3 visits stored with one embedding call, got %+v and %d calls (err %v)", report, len(embedder.Inputs), err)
	}
}

// SnapshotFake is a collector that emits a directory's complete state.
type SnapshotFake struct {
	FakeCollector
	Root string
}

func (s SnapshotFake) SnapshotRoot() string {
	return s.Root
}

func fileAt(path string, content string) event.Event {
	return event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile, Timestamp: time.Unix(1, 0),
		Content: content, Metadata: event.File{Path: path, ModifiedAt: time.Unix(1, 0)}.Metadata()}
}

func TestSnapshotMarksMissingFiles(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.MissingFiles = 2
	collector := SnapshotFake{FakeCollector: FakeCollector{Events: []event.Event{fileAt("/notas/a.md", "a")}}, Root: "/notas"}
	report, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), collector, nil)
	if err != nil || report.Removed != 2 || len(store.MarkedRoots) != 1 || store.MarkedRoots[0] != "/notas" || !store.LastPresent["/notas/a.md"] {
		t.Fatalf("expected the root checked with the collected path, got %+v %v %v (err %v)", report, store.MarkedRoots, store.LastPresent, err)
	}
}

// Only a complete snapshot says a file is gone; a failed or partial run
// must not flag anything.
func TestNonSnapshotOrFailedRunsMarkNothing(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(), nil)
	failing := SnapshotFake{FakeCollector: FakeCollector{Events: []event.Event{fileAt("/notas/a.md", "a")}}, Root: "/notas"}
	newTestPipeline(store, &testfakes.FakeEmbedder{FailWith: errors.New("gpu")}).Run(context.Background(), failing, nil)
	if len(store.MarkedRoots) != 0 {
		t.Fatalf("expected no root marked, got %v", store.MarkedRoots)
	}
}

func longNote() event.Event {
	return event.Event{UID: "nota", Source: event.SourceFile, Timestamp: time.Unix(1, 0),
		Content: "arquitetura.md\n" + strings.Repeat("Parágrafo sobre filas e custos. ", 120)}
}

// Regression: a long note was one vector cut at the embedder's context, so
// its second half could never be found.
func TestRunEmbedsEachChunkOfALongEvent(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	newTestPipeline(store, embedder).Run(context.Background(), FakeCollector{Events: []event.Event{longNote()}}, nil)
	chunks := store.Chunks["nota"]
	if len(chunks) < 3 || len(embedder.Inputs) != len(chunks) || chunks[len(chunks)-1].End != len(longNote().Content) {
		t.Fatalf("expected one embedding per chunk covering the text, got %d chunks and %d calls", len(chunks), len(embedder.Inputs))
	}
	if !strings.HasPrefix(embedder.Inputs[1], "doc: ") || chunks[1].Ordinal != 1 {
		t.Fatalf("expected prefixed, ordered chunks, got %q and %+v", embedder.Inputs[1][:10], chunks[1])
	}
}

func TestRunReusesAllChunksOfIdenticalLongText(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	copy := longNote()
	copy.UID = "copia"
	newTestPipeline(store, embedder).Run(context.Background(), FakeCollector{Events: []event.Event{longNote(), copy}}, nil)
	if len(store.Chunks["copia"]) != len(store.Chunks["nota"]) || len(embedder.Inputs) != len(store.Chunks["nota"]) {
		t.Fatalf("expected the copy to reuse every chunk, got %d chunks and %d calls", len(store.Chunks["copia"]), len(embedder.Inputs))
	}
}

// AuthoredFake reports the identities of its repository.
type AuthoredFake struct {
	FakeCollector
}

func (AuthoredFake) CommitAuthorship() (string, []string) {
	return "/src/api", []string{"ana@x.io"}
}

func TestAuthoredCollectorMarksStoredCommits(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), AuthoredFake{}, nil)
	if len(store.AuthorshipMarks) != 1 || store.AuthorshipMarks[0] != "/src/api" {
		t.Fatalf("expected the repository marked, got %v", store.AuthorshipMarks)
	}
}
