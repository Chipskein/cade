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
	"github.com/chipskein/cade/internal/testcheck"
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
	if _, err := pipeline.Run(context.Background(), twoEvents(), nil); err != nil {
		t.Fatal(err)
	}
	report, _ := pipeline.Run(context.Background(), twoEvents(), nil)
	if report != (Report{Collected: 2, AlreadyStored: 2}) || len(store.Events) != 2 {
		t.Fatalf("expected re-run to skip both, got %+v with %d stored", report, len(store.Events))
	}
}

func TestRunDoesNotRestoreForgottenUID(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Forgotten = map[string]bool{"a": true}
	report, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), FakeCollector{Events: []event.Event{{UID: "a", Source: event.SourceGit, Timestamp: time.Unix(1, 0), Content: "forgotten"}}}, nil)
	if err != nil || len(store.Events) != 0 || report.AlreadyStored != 1 {
		t.Fatalf("forgotten event returned: %+v, %v, %+v", store.Events, report, err)
	}
}

func TestRetentionDeletesOldEventsAfterIngest(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	pipeline := newTestPipeline(store, &testfakes.FakeEmbedder{}).WithClock(func() time.Time { return now }).WithRetention(map[event.Source]int{event.SourceGit: 30})
	collector := FakeCollector{Events: []event.Event{{UID: "old", Source: event.SourceGit, Timestamp: now.AddDate(0, 0, -31), Content: "old event"}, {UID: "recent", Source: event.SourceGit, Timestamp: now.AddDate(0, 0, -2), Content: "recent event"}}}
	if _, err := pipeline.Run(context.Background(), collector, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.Events) != 1 || store.Events[0].UID != "recent" {
		t.Fatalf("retention kept wrong events: %+v", store.Events)
	}
}

func TestRunSkipsEmbeddingForKnownEvents(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	pipeline := newTestPipeline(store, embedder)
	if _, err := pipeline.Run(context.Background(), twoEvents(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Run(context.Background(), twoEvents(), nil); err != nil {
		t.Fatal(err)
	}
	if len(embedder.Inputs) != 1 {
		t.Fatalf("expected a single embedding call overall, got %d", len(embedder.Inputs))
	}
}

func TestRunEmbedsWithDocumentPrefix(t *testing.T) {
	embedder := &testfakes.FakeEmbedder{}
	if _, err := newTestPipeline(testfakes.NewFakeEventStore(), embedder).Run(context.Background(), twoEvents(), nil); err != nil {
		t.Fatal(err)
	}
	if embedder.Inputs[0] != "doc: fix bug" {
		t.Fatalf("expected prefixed text, got %q", embedder.Inputs[0])
	}
}

func TestRunStoresEventWithoutContentWithoutEmbedding(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	if _, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(), nil); err != nil {
		t.Fatal(err)
	}
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
	_, err := newTestPipeline(testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(),
		func(report Report) { seen = append(seen, report) })
	testcheck.NoError(t, err)
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
	if _, err := pipeline.Run(context.Background(), FakeCollector{Events: []event.Event{teamsVersion("deploy às 18h", "100")}}, nil); err != nil {
		t.Fatal(err)
	}
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
	if _, err := pipeline.Run(context.Background(), versions, nil); err != nil {
		t.Fatal(err)
	}
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

// imageVersion is the same file (revision 1) at a given caption status.
func imageVersion(status event.CaptionStatus) event.Event {
	metadata := event.Metadata{event.RevisionKey: "1"}
	for key, value := range (event.Image{Status: status}).Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: "img", Source: event.SourceFile, Content: "erro.png", Metadata: metadata}
}

// An unchanged image keeps its revision, so without this rule one that
// waited for the per-run limit would never get its description.
func TestReplacesAnUnchangedImageOnlyWhenItsCaptionAdvances(t *testing.T) {
	cases := []struct {
		incoming, stored event.CaptionStatus
		expected         bool
	}{
		{event.CaptionDescribed, event.CaptionPending, true},
		{event.CaptionDescribed, event.CaptionNone, true},
		{event.CaptionUnreadable, event.CaptionPending, true},
		{event.CaptionPending, event.CaptionNone, false},
		{event.CaptionDescribed, event.CaptionDescribed, false},
		{event.CaptionPending, event.CaptionDescribed, false},
		{event.CaptionNone, event.CaptionDescribed, false},
	}
	for _, c := range cases {
		if got := replaces(imageVersion(c.incoming), imageVersion(c.stored)); got != c.expected {
			t.Errorf("%q over stored %q: expected %v, got %v", c.incoming, c.stored, c.expected, got)
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
	if _, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), twoEvents(), nil); err != nil {
		t.Fatal(err)
	}
	failing := SnapshotFake{FakeCollector: FakeCollector{Events: []event.Event{fileAt("/notas/a.md", "a")}}, Root: "/notas"}
	_, err := newTestPipeline(store, &testfakes.FakeEmbedder{FailWith: errors.New("gpu")}).Run(context.Background(), failing, nil)
	if err == nil || len(store.MarkedRoots) != 0 {
		t.Fatalf("expected a failed run and no root marked, got %v (err %v)", store.MarkedRoots, err)
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
	if _, err := newTestPipeline(store, embedder).Run(context.Background(), FakeCollector{Events: []event.Event{longNote()}}, nil); err != nil {
		t.Fatal(err)
	}
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
	if _, err := newTestPipeline(store, embedder).Run(context.Background(), FakeCollector{Events: []event.Event{longNote(), copy}}, nil); err != nil {
		t.Fatal(err)
	}
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
	if _, err := newTestPipeline(store, &testfakes.FakeEmbedder{}).Run(context.Background(), AuthoredFake{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.AuthorshipMarks) != 1 || store.AuthorshipMarks[0] != "/src/api" {
		t.Fatalf("expected the repository marked, got %v", store.AuthorshipMarks)
	}
}

func TestReplaceStoredMasksAndReembeds(t *testing.T) {
	store, embedder := testfakes.NewFakeEventStore(), &testfakes.FakeEmbedder{}
	pipeline := newTestPipeline(store, embedder)
	stored := event.Event{UID: "img", Source: event.SourceFile, Content: "erro.png\nantigo", Metadata: event.Metadata{}}
	store.Events = []event.Event{stored}
	stored.Content = "erro.png\nexport TOKEN=ghp_0123456789abcdefghijABCDEFGHIJ"
	if err := pipeline.ReplaceStored(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	if got := store.Events[0].Content; !strings.Contains(got, "[redacted:github-token]") || len(embedder.Inputs) != 1 || store.Updated[0] != "img" {
		t.Fatalf("expected the masked text stored and embedded, got %q with %d embeds", got, len(embedder.Inputs))
	}
}

// InterruptedCollector emits its first Emitted events, then fails as a
// Ctrl-C would, mid-batch.
type InterruptedCollector struct {
	Events  []event.Event
	Emitted int
}

func (c InterruptedCollector) CollectEvents(ctx context.Context, emit EmitFunc) error {
	if err := (FakeCollector{Events: c.Events[:c.Emitted]}).CollectEvents(ctx, emit); err != nil {
		return err
	}
	return context.Canceled
}

func fiveEvents() FakeCollector {
	var events []event.Event
	for i, uid := range []string{"a", "b", "c", "d", "e"} {
		events = append(events, event.Event{UID: uid, Source: event.SourceGit, Timestamp: time.Unix(int64(i), 0), Content: "commit " + uid})
	}
	return FakeCollector{Events: events}
}

func newBatchingPipeline(store *testfakes.FakeEventStore, eventsPerBatch int) *Pipeline {
	pipeline := newTestPipeline(store, &testfakes.FakeEmbedder{})
	pipeline.eventsPerBatch = eventsPerBatch
	return pipeline
}

func storedUIDs(store *testfakes.FakeEventStore) []string {
	var uids []string
	for _, ev := range store.Events {
		uids = append(uids, ev.UID+"="+ev.Content)
	}
	return uids
}

func TestRunCommitsEachFullBatchAndTheRest(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	_, err := newBatchingPipeline(store, 2).Run(context.Background(), fiveEvents(), nil)
	testcheck.NoError(t, err)
	if store.BatchesBegun != 3 || store.Commits != 3 || store.Rollbacks != 0 || len(store.Events) != 5 {
		t.Fatalf("expected 3 batches (2+2+1) committed, got begun=%d commits=%d rollbacks=%d stored=%d",
			store.BatchesBegun, store.Commits, store.Rollbacks, len(store.Events))
	}
}

func TestRunWithNothingCollectedOpensNoBatch(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	_, err := newBatchingPipeline(store, 2).Run(context.Background(), FakeCollector{}, nil)
	if err != nil || store.BatchesBegun != 0 {
		t.Fatalf("expected no batch for an empty collection, got %d (err %v)", store.BatchesBegun, err)
	}
}

func TestRunInterruptedMidBatchLosesOnlyThatBatch(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	interrupted := InterruptedCollector{Events: fiveEvents().Events, Emitted: 3}
	_, err := newBatchingPipeline(store, 2).Run(context.Background(), interrupted, nil)
	if !errors.Is(err, context.Canceled) || store.Rollbacks != 1 || strings.Join(storedUIDs(store), ",") != "a=commit a,b=commit b" {
		t.Fatalf("expected the first batch kept and the open one rolled back, got %v (err %v, rollbacks %d)", storedUIDs(store), err, store.Rollbacks)
	}
}

// Regression for #51: batching must not lose what an interruption rolled
// back; running again stores the same events as a run never interrupted.
func TestRunAfterInterruptionStoresTheSameEventsAsAnUninterruptedRun(t *testing.T) {
	uninterrupted := testfakes.NewFakeEventStore()
	_, err := newBatchingPipeline(uninterrupted, 2).Run(context.Background(), fiveEvents(), nil)
	testcheck.NoError(t, err)
	resumed := testfakes.NewFakeEventStore()
	pipeline := newBatchingPipeline(resumed, 2)
	if _, err := pipeline.Run(context.Background(), InterruptedCollector{Events: fiveEvents().Events, Emitted: 3}, nil); err == nil {
		t.Fatal("expected the interrupted run to fail")
	}
	report, err := pipeline.Run(context.Background(), fiveEvents(), nil)
	testcheck.NoError(t, err)
	want, got := strings.Join(storedUIDs(uninterrupted), ","), strings.Join(storedUIDs(resumed), ",")
	if got != want || report.Inserted != 3 || report.AlreadyStored != 2 {
		t.Fatalf("expected %s after resuming (3 inserted, 2 already stored), got %s with %+v", want, got, report)
	}
}
