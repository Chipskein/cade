package retrievalsuite

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

const suiteDir = "../../testdata/queries/retrieval"

func loadShippedSuite(t *testing.T, set string) Suite {
	t.Helper()
	corpusFile, err := os.Open(filepath.Join(suiteDir, "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer corpusFile.Close()
	casesFile, err := os.Open(filepath.Join(suiteDir, set+".json"))
	if err != nil {
		t.Fatal(err)
	}
	defer casesFile.Close()
	corpus, err := LoadCorpus(corpusFile)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := LoadCases(casesFile)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := NewSuite(corpus, cases)
	if err != nil {
		t.Fatalf("%s: %v", set, err)
	}
	return suite
}

func TestShippedSetsAreValidAndDisjoint(t *testing.T) {
	calibration, test := loadShippedSuite(t, "calibration"), loadShippedSuite(t, "test")
	if len(calibration.Events) < 280 || len(calibration.Cases) < 20 || len(test.Cases) < 20 {
		t.Fatalf("expected a real corpus and two sets, got %d events, %d and %d cases", len(calibration.Events), len(calibration.Cases), len(test.Cases))
	}
	seen := map[string]bool{}
	for _, c := range calibration.Cases {
		seen[c.Question] = true
	}
	for _, c := range test.Cases {
		if seen[c.Question] {
			t.Fatalf("question %q is in both sets; the test set must stay unseen while tuning", c.Question)
		}
	}
}

func TestNewSuiteRejectsUnknownGroup(t *testing.T) {
	corpus := Corpus{Events: []CorpusEvent{{ID: "a-1", Group: "a"}}}
	if _, err := NewSuite(corpus, CaseSet{Cases: []Case{{Question: "q", Relevant: []string{"a-1"}}}}); err == nil || !strings.Contains(err.Error(), `"a-1"`) {
		t.Fatalf("expected an error naming the group, got %v", err)
	}
	if _, err := NewSuite(corpus, CaseSet{Cases: []Case{{Question: "q", Direction: "para"}}}); err == nil || !strings.Contains(err.Error(), `"para"`) {
		t.Fatalf("expected an error naming the direction, got %v", err)
	}
}

func TestLoadersRequireContent(t *testing.T) {
	if _, err := LoadCorpus(strings.NewReader(`{"now": "2026-09-26T10:00:00Z", "events": []}`)); err == nil {
		t.Error("expected an empty corpus rejected")
	}
	if _, err := LoadCases(strings.NewReader(`{"cases": []}`)); err == nil {
		t.Error("expected an empty case set rejected")
	}
}

func TestCaseQueryResolvesLikeAsk(t *testing.T) {
	suite := loadShippedSuite(t, "test")
	query := Case{Question: "commits de ontem sobre auth", Period: "ontem", Source: event.SourceGit, Topic: "auth", Direction: "enviadas"}.Query(suite.Now)
	if query.Days.String() != "2026-09-25" || query.SemanticText != "auth" || query.Criteria.Direction != listing.Sent {
		t.Fatalf("unexpected query %+v", query)
	}
}

func TestCaseResultMetricsCountGroups(t *testing.T) {
	result := CaseResult{Relevant: []string{"a", "b"}, Retrieved: []string{"x", "b", "b", "y"}}
	if result.Recall() != 0.5 || result.ReciprocalRank() != 0.5 || result.Passed() || result.Missing()[0] != "a" || result.Repeats() != 1 {
		t.Fatalf("unexpected metrics for %+v", result)
	}
	if !(CaseResult{}).Passed() || (CaseResult{Retrieved: []string{"x"}}).Passed() {
		t.Fatal("expected an unanswerable case to pass only when nothing is retrieved")
	}
}

func TestScoreboardAveragesAndFloors(t *testing.T) {
	board := Scoreboard{Results: []CaseResult{
		{Relevant: []string{"a"}, Retrieved: []string{"a", "a"}},
		{Relevant: []string{"b"}, Retrieved: []string{"x"}},
		{Retrieved: []string{"x"}},
	}}
	if board.MeanRecall() != 0.5 || board.MRR() != 0.5 || board.Rejection() != 0 || board.Redundancy() != 0.25 {
		t.Fatalf("unexpected averages %.2f %.2f %.2f %.2f", board.MeanRecall(), board.MRR(), board.Rejection(), board.Redundancy())
	}
	below := board.BelowMinimum(CaseSet{MinimumRecall: 0.5, MinimumMRR: 0.6})
	if len(below) != 1 || !strings.HasPrefix(below[0], "mrr") {
		t.Fatalf("expected only mrr below its floor, got %v", below)
	}
}

func TestWriteReportExplainsFailures(t *testing.T) {
	board := Scoreboard{Results: []CaseResult{
		{Question: "q1", Relevant: []string{"a"}, Retrieved: []string{"x"}},
		{Question: "q2", Retrieved: []string{"y"}},
	}}
	var out strings.Builder
	board.WriteReport(&out)
	if !strings.Contains(out.String(), "faltou: a; veio: x") || !strings.Contains(out.String(), "deveria vir vazio, veio: y") || !strings.Contains(out.String(), "redundância") {
		t.Fatalf("unexpected report:\n%s", out.String())
	}
}

func TestCalibrateUsesUnscopedClosestDistances(t *testing.T) {
	board := Scoreboard{Results: []CaseResult{
		{Relevant: []string{"a"}, Retrieved: []string{"a"}, Distances: []float64{0.5}},
		{Relevant: []string{"b"}, Retrieved: []string{"b"}, Distances: []float64{0.58}},
		{Retrieved: []string{"x"}, Distances: []float64{0.66}},
		{Retrieved: []string{"y"}, Distances: []float64{0.1}, Scoped: true},
	}}
	calibration := Calibrate(board)
	if calibration.Answerable != 2 || calibration.Unanswerable != 1 || !calibration.Separates() || math.Abs(calibration.Suggested()-0.62) > 1e-9 {
		t.Fatalf("unexpected calibration %+v", calibration)
	}
	var out strings.Builder
	calibration.WriteReport(&out, 0.62)
	if !strings.Contains(out.String(), "sugerido 0.620") {
		t.Fatalf("unexpected report %q", out.String())
	}
}

func TestCalibrationReportsOverlap(t *testing.T) {
	calibration := Calibration{Answerable: 1, Unanswerable: 1, AnswerableMax: 0.7, UnanswerableMin: 0.6}
	var out strings.Builder
	calibration.WriteReport(&out, 0.62)
	if calibration.Separates() || !strings.Contains(out.String(), "se sobrepõem") {
		t.Fatalf("expected the overlap reported, got %q", out.String())
	}
}

func TestDistractorsAreDeterministicAndFillToTotal(t *testing.T) {
	suite := loadShippedSuite(t, "test")
	first, second := suite.WithDistractors(400), suite.WithDistractors(400)
	if len(first.events()) != 400 || first.events()[399].Content != second.events()[399].Content {
		t.Fatalf("expected 400 identical events, got %d", len(first.events()))
	}
	if len(suite.WithDistractors(10).events()) != len(suite.Events) {
		t.Fatal("a total below the corpus size adds nothing")
	}
}

func TestCachingEmbedderPersistsVectors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache", "model.gob")
	first := &testfakes.FakeEmbedder{}
	cache, _ := NewCachingEmbedder(first, path)
	cache.Embed("a")
	cache.Embed("a")
	if err := cache.Save(); err != nil || len(first.Inputs) != 1 {
		t.Fatalf("expected one real embedding and a saved cache, got %d (err %v)", len(first.Inputs), err)
	}
	second := &testfakes.FakeEmbedder{}
	reloaded, err := NewCachingEmbedder(second, path)
	if vector, _ := reloaded.Embed("a"); err != nil || len(second.Inputs) != 0 || len(vector) == 0 {
		t.Fatalf("expected the vector from disk, got %v (err %v, %d embeds)", vector, err, len(second.Inputs))
	}
}

func TestRunMapsHitsToGroups(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.SearchResults = []storage.ScoredEvent{{Event: event.Event{UID: "p-2"}, Distance: 0.1}, {Event: event.Event{UID: "p-1"}, Distance: 0.2}}
	corpus := Corpus{Events: []CorpusEvent{{ID: "p-1", Group: "p", Source: event.SourceBrowser, Content: "Página"}, {ID: "p-2", Group: "p", Source: event.SourceBrowser, Content: "Página"}}}
	suite, _ := NewSuite(corpus, CaseSet{Cases: []Case{{Question: "página", Relevant: []string{"p"}}}})
	deps := Dependencies{Store: store, Embedder: &testfakes.FakeEmbedder{}, Settings: rag.Settings{TopK: 5, MaxDistance: 0.5},
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	board, err := Run(context.Background(), deps, suite, nil)
	if err != nil || len(store.Events) != 2 || !board.Results[0].Passed() || board.Results[0].Repeats() != 1 {
		t.Fatalf("expected both visits as one group with one repeat, got %+v (err %v)", board, err)
	}
}
