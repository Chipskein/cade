package retrievalsuite

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

const shippedSuitePath = "../../testdata/queries/retrieval.json"

func loadShippedSuite(t *testing.T) Suite {
	t.Helper()
	file, err := os.Open(shippedSuitePath)
	if err != nil {
		t.Fatalf("open %s: %v", shippedSuitePath, err)
	}
	defer file.Close()
	suite, err := Load(file)
	if err != nil {
		t.Fatalf("load %s: %v", shippedSuitePath, err)
	}
	return suite
}

func TestShippedSuiteIsValid(t *testing.T) {
	suite := loadShippedSuite(t)
	if len(suite.Events) < 30 || len(suite.Cases) < 20 {
		t.Fatalf("expected a real corpus, got %d events and %d cases", len(suite.Events), len(suite.Cases))
	}
}

func TestLoadRejectsUnknownRelevantID(t *testing.T) {
	raw := `{"now": "2026-09-26T10:00:00Z", "events": [{"id": "a"}], "cases": [{"question": "q", "relevant": ["zz"]}]}`
	if _, err := Load(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), `"zz"`) {
		t.Fatalf("expected an error naming the id, got %v", err)
	}
}

func TestLoadRejectsUnknownDirection(t *testing.T) {
	raw := `{"now": "2026-09-26T10:00:00Z", "events": [{"id": "a"}], "cases": [{"question": "q", "direction": "para"}]}`
	if _, err := Load(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), `"para"`) {
		t.Fatalf("expected an error naming the direction, got %v", err)
	}
}

func TestCaseQueryResolvesLikeAsk(t *testing.T) {
	suite := loadShippedSuite(t)
	query := Case{Question: "commits de ontem sobre auth", Period: "ontem", Source: event.SourceGit, Topic: "auth", Direction: "enviadas"}.Query(suite.Now)
	if query.Days.String() != "2026-09-25" || query.SemanticText != "auth" || query.Criteria.Direction != listing.Sent {
		t.Fatalf("unexpected query %+v", query)
	}
}

func TestCaseResultMetrics(t *testing.T) {
	result := CaseResult{Relevant: []string{"a", "b"}, Retrieved: []string{"x", "b", "y"}}
	if result.Recall() != 0.5 || result.ReciprocalRank() != 0.5 || result.Passed() || result.Missing()[0] != "a" {
		t.Fatalf("unexpected metrics for %+v", result)
	}
	if !(CaseResult{}).Passed() || (CaseResult{Retrieved: []string{"x"}}).Passed() {
		t.Fatal("expected an unanswerable case to pass only when nothing is retrieved")
	}
}

func TestScoreboardAveragesAndFloors(t *testing.T) {
	board := Scoreboard{Results: []CaseResult{
		{Relevant: []string{"a"}, Retrieved: []string{"a"}},
		{Relevant: []string{"b"}, Retrieved: []string{"x"}},
		{Retrieved: []string{"x"}},
	}}
	if board.MeanRecall() != 0.5 || board.MRR() != 0.5 || board.Rejection() != 0 {
		t.Fatalf("unexpected averages %.2f %.2f %.2f", board.MeanRecall(), board.MRR(), board.Rejection())
	}
	below := board.BelowMinimum(Suite{MinimumRecall: 0.5, MinimumMRR: 0.6, MinimumRejection: 0})
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
	if !strings.Contains(out.String(), "faltou: a; veio: x") || !strings.Contains(out.String(), "deveria vir vazio, veio: y") {
		t.Fatalf("unexpected report:\n%s", out.String())
	}
}

func TestRunIngestsCorpusAndRetrieves(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.SearchResults = []storage.ScoredEvent{{Event: event.Event{UID: "a"}, Distance: 0.1}}
	suite := Suite{Events: []CorpusEvent{{ID: "a", Source: event.SourceGit, Content: "Corrige login"}},
		Cases: []Case{{Question: "login", Relevant: []string{"a"}}}}
	deps := Dependencies{Store: store, Embedder: &testfakes.FakeEmbedder{}, Settings: rag.Settings{TopK: 5, MaxDistance: 0.5},
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	var progress []int
	board, err := Run(context.Background(), deps, suite, func(done, total int, _ CaseResult) { progress = append(progress, done, total) })
	if err != nil || len(store.Events) != 1 || len(board.Results) != 1 || !board.Results[0].Passed() || len(progress) != 2 {
		t.Fatalf("expected the corpus stored and the case passed, got %+v (err %v, %d stored)", board, err, len(store.Events))
	}
}
