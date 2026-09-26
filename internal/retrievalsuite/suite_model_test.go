package retrievalsuite

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage/sqlitestore"
)

// embeddingModelEnv points at the real GGUF model (Makefile targets
// test-models and eval-retrieval); without it the suite is skipped.
const embeddingModelEnv = "CADE_TEST_EMBEDDING_MODEL"

// TestRetrievalSuiteWithModel ingests the corpus into a real SQLite store
// with the real embedder and fails when a metric drops below its floor.
func TestRetrievalSuiteWithModel(t *testing.T) {
	deps := realDependencies(t)
	suite := loadShippedSuite(t)
	board, err := Run(context.Background(), deps, suite, func(done, total int, result CaseResult) {
		t.Logf("[%2d/%d] %s %s", done, total, passMark(result), result.Question)
	})
	if err != nil {
		t.Fatal(err)
	}
	var report strings.Builder
	board.WriteReport(&report)
	t.Log("\n" + report.String())
	if below := board.BelowMinimum(suite); len(below) > 0 {
		t.Fatalf("metrics below the suite floors: %v", below)
	}
}

func realDependencies(t *testing.T) Dependencies {
	t.Helper()
	path := os.Getenv(embeddingModelEnv)
	if path == "" {
		t.Skipf("%s not set; skipping the retrieval suite", embeddingModelEnv)
	}
	embedder, err := llamacpp.LoadEmbedder(llamacpp.ModelOptions{Path: path, GPULayers: -1})
	if err != nil {
		t.Fatalf("load embedder: %v", err)
	}
	t.Cleanup(func() { embedder.Close() })
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "suite.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	defaults := config.Defaults()
	settings := rag.Settings{TopK: defaults.Retrieval.TopK, MaxDistance: defaults.Retrieval.MaxDistance,
		MaxBestDistance: defaults.Retrieval.MaxBestDistance, QueryPrefix: defaults.Embedding.QueryPrefix}
	return Dependencies{Store: store, Embedder: embedder, Settings: settings, DocumentPrefix: defaults.Embedding.DocumentPrefix,
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
}

func passMark(result CaseResult) string {
	if result.Passed() {
		return "ok"
	}
	return "✗ "
}
