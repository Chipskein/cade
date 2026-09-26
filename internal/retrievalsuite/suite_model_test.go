package retrievalsuite

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage/sqlitestore"
)

// embeddingModelEnv points at the real GGUF model (Makefile targets
// test-models and eval-retrieval); without it these are skipped.
// scaleEnv lists corpus sizes for the scale curve ("1000,10000").
const (
	embeddingModelEnv = "CADE_TEST_EMBEDDING_MODEL"
	scaleEnv          = "CADE_EVAL_SCALE"
)

// TestRetrievalCalibrationWithModel runs the calibration set with the
// distance gates off and reports where they should be. No floors: this
// set is for tuning, and the test set checks the result.
func TestRetrievalCalibrationWithModel(t *testing.T) {
	embedder := loadEmbedder(t)
	deps := dependencies(t, embedder)
	deps.Settings.MaxDistance, deps.Settings.MaxBestDistance = 2, 0
	board, err := Run(context.Background(), deps, loadShippedSuite(t, "calibration"), logCase(t))
	if err != nil {
		t.Fatal(err)
	}
	var report strings.Builder
	Calibrate(board).WriteReport(&report, config.Defaults().Retrieval.MaxBestDistance)
	t.Log("\n" + report.String())
}

// TestRetrievalSuiteWithModel checks the test set, never used for tuning,
// against its floors.
func TestRetrievalSuiteWithModel(t *testing.T) {
	suite := loadShippedSuite(t, "test")
	board, err := Run(context.Background(), dependencies(t, loadEmbedder(t)), suite, logCase(t))
	if err != nil {
		t.Fatal(err)
	}
	var report strings.Builder
	board.WriteReport(&report)
	t.Log("\n" + report.String())
	if below := board.BelowMinimum(suite.CaseSet); len(below) > 0 {
		t.Fatalf("metrics below the test set floors: %v", below)
	}
}

// TestRetrievalScaleWithModel reruns the test set on corpora grown with
// distractors to each size in CADE_EVAL_SCALE, printing one line per size.
func TestRetrievalScaleWithModel(t *testing.T) {
	sizes := os.Getenv(scaleEnv)
	if sizes == "" {
		t.Skipf("%s not set; skipping the scale curve", scaleEnv)
	}
	embedder := loadEmbedder(t)
	cached, err := NewCachingEmbedder(embedder, embeddingCachePath(t, embedder.ContextTokens()))
	if err != nil {
		t.Fatal(err)
	}
	defer cached.Save()
	for _, size := range strings.Split(sizes, ",") {
		total, err := strconv.Atoi(strings.TrimSpace(size))
		if err != nil {
			t.Fatalf("%s entry %q, expected an integer", scaleEnv, size)
		}
		board, err := Run(context.Background(), dependencies(t, cached), loadShippedSuite(t, "test").WithDistractors(total), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("eventos=%-7d recall=%.2f mrr=%.2f rejeição=%.2f redundância=%.2f", total, board.MeanRecall(), board.MRR(), board.Rejection(), board.Redundancy())
	}
}

func loadEmbedder(t *testing.T) *llamacpp.Embedder {
	t.Helper()
	path := os.Getenv(embeddingModelEnv)
	if path == "" {
		t.Skipf("%s not set; skipping the retrieval suite", embeddingModelEnv)
	}
	embedder, err := llamacpp.LoadEmbedder(llamacpp.ModelOptions{Path: path, ContextTokens: config.Defaults().Embedding.ContextTokens, GPULayers: -1})
	if err != nil {
		t.Fatalf("load embedder: %v", err)
	}
	t.Cleanup(func() { embedder.Close() })
	return embedder
}

// embeddingCachePath keys the cache by model file and effective context:
// vectors of two models, or of one model cut at two lengths, are not
// interchangeable.
func embeddingCachePath(t *testing.T, contextTokens int) string {
	t.Helper()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("%s.ctx%d.gob", filepath.Base(os.Getenv(embeddingModelEnv)), contextTokens)
	return filepath.Join(cacheDir, "cade", "eval", name)
}

// dependencies uses a fresh SQLite store and the default settings.
func dependencies(t *testing.T, embedder llm.Embedder) Dependencies {
	t.Helper()
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

func logCase(t *testing.T) CaseDone {
	return func(done, total int, result CaseResult) {
		mark := "ok"
		if !result.Passed() {
			mark = "✗ "
		}
		t.Logf("[%2d/%d] %s %s", done, total, mark, result.Question)
	}
}
