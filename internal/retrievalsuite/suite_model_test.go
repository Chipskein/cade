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
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/evalimages"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage/sqlitestore"
	"github.com/chipskein/cade/internal/testcheck"
)

// embeddingModelEnv points at the real GGUF model (mage targets
// testModels and evalRetrieval); without it these are skipped.
// scaleEnv lists corpus sizes for the scale curve ("1000,10000").
const (
	embeddingModelEnv = "CADE_TEST_EMBEDDING_MODEL"
	scaleEnv          = "CADE_EVAL_SCALE"
	topKEnv           = "CADE_EVAL_TOP_K"
)

var retrievalSweepTopK = []int{4, 6, 8, 12}
var retrievalSweepMaxDistance = []float64{0.68, 0.72, 0.76}
var retrievalSweepMaxBestDistance = []float64{0.60, 0.61, 0.63}

// TestRetrievalSweepWithModel crosses top_k with both distance gates on
// the calibration and test sets (phase 17, docs/BENCHMARKS.md).
func TestRetrievalSweepWithModel(t *testing.T) {
	warmFixtureCaptions(t)
	cached := cachedEmbedder(t)
	calibration, testSuite := loadCaptionedSuite(t, "calibration"), loadCaptionedSuite(t, "test")
	deps := dependencies(t, cached)
	for _, topK := range retrievalSweepTopK {
		deps.Settings.TopK = topK
		for _, maxDistance := range retrievalSweepMaxDistance {
			for _, maxBest := range retrievalSweepMaxBestDistance {
				deps.Settings.MaxDistance, deps.Settings.MaxBestDistance = maxDistance, maxBest
				calibrationBoard, err := Run(context.Background(), deps, calibration, nil)
				if err != nil {
					t.Fatal(err)
				}
				board, err := Run(context.Background(), deps, testSuite, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("top_k=%-2d max_distance=%.3f max_best_distance=%.3f calib(recall=%.2f,mrr=%.2f,rejeição=%.2f) teste(recall=%.2f,mrr=%.2f,rejeição=%.2f)",
					topK, maxDistance, maxBest, calibrationBoard.MeanRecall(), calibrationBoard.MRR(), calibrationBoard.Rejection(), board.MeanRecall(), board.MRR(), board.Rejection())
			}
		}
	}
}

// TestRetrievalCalibrationWithModel runs the calibration set with the
// distance gates off and reports where they should be. No floors: this
// set is for tuning, and the test set checks the result.
func TestRetrievalCalibrationWithModel(t *testing.T) {
	warmFixtureCaptions(t)
	embedder := loadEmbedder(t)
	deps := dependencies(t, embedder)
	deps.Settings.MaxDistance, deps.Settings.MaxBestDistance = 2, 0
	board, err := Run(context.Background(), deps, loadCaptionedSuite(t, "calibration"), logCase(t))
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
	warmFixtureCaptions(t)
	suite := loadCaptionedSuite(t, "test")
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
	sizes := scaleSizes(t)
	if len(sizes) == 0 {
		t.Skipf("%s not set; skipping the scale curve", scaleEnv)
	}
	warmFixtureCaptions(t)
	cached := cachedEmbedder(t)
	for _, total := range sizes {
		deps := dependencies(t, cached)
		deps.Settings.TopK = evaluationTopK(t)
		board, err := Run(context.Background(), deps, loadCaptionedSuite(t, "test").WithDistractors(total), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("eventos=%-7d recall=%.2f mrr=%.2f rejeição=%.2f redundância=%.2f", total, board.MeanRecall(), board.MRR(), board.Rejection(), board.Redundancy())
	}
}

const (
	rerankerModelEnv = "CADE_TEST_RERANKER_MODEL"
	// rerankCandidates is how many hybrid-search hits the reranker orders
	// before the cut to top_k (phase 17).
	rerankCandidates = 30
	// rerankContextTokens fits a question and one 1,200-character chunk.
	rerankContextTokens = 1024
)

// TestRetrievalRerankWithModel is the phase 17 experiment: the test set
// (and each CADE_EVAL_SCALE size) with and without reranking
// rerankCandidates hits down to the default top_k, with the time the
// reranker takes per question.
func TestRetrievalRerankWithModel(t *testing.T) {
	warmFixtureCaptions(t)
	reranker := loadReranker(t)
	cached := cachedEmbedder(t)
	sizes := append([]int{0}, scaleSizes(t)...)
	for _, size := range sizes {
		suite := loadCaptionedSuite(t, "test").WithDistractors(size)
		for _, rerank := range []*Reranking{nil, {Scorer: reranker, Keep: config.Defaults().Retrieval.TopK}} {
			deps := dependencies(t, cached)
			if rerank != nil {
				deps.Settings.TopK, deps.Rerank = rerankCandidates, &Reranking{Scorer: &timedScorer{scorer: rerank.Scorer}, Keep: rerank.Keep}
			}
			board, err := Run(context.Background(), deps, suite, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("eventos=%-6d rerank=%-5t recall=%.2f mrr=%.2f rejeição=%.2f %s", size, rerank != nil,
				board.MeanRecall(), board.MRR(), board.Rejection(), rerankCost(deps.Rerank, len(suite.Cases)))
		}
	}
}

// timedScorer adds up the time the reranker spends.
type timedScorer struct {
	scorer RelevanceScorer
	spent  time.Duration
	pairs  int
}

func (s *timedScorer) Score(query, document string) (float32, error) {
	started := time.Now()
	defer func() { s.spent += time.Since(started); s.pairs++ }()
	return s.scorer.Score(query, document)
}

func rerankCost(rerank *Reranking, questions int) string {
	if rerank == nil {
		return ""
	}
	timed := rerank.Scorer.(*timedScorer)
	return fmt.Sprintf("rerank=%v/pergunta (%d pares)", timed.spent/time.Duration(max(questions, 1)), timed.pairs)
}

func loadReranker(t *testing.T) *llamacpp.Reranker {
	t.Helper()
	path := os.Getenv(rerankerModelEnv)
	if path == "" {
		t.Skipf("%s not set; skipping the reranking experiment", rerankerModelEnv)
	}
	reranker, err := llamacpp.LoadReranker(llamacpp.ModelOptions{Path: path, ContextTokens: rerankContextTokens,
		GPULayers: config.Defaults().Embedding.GPULayers})
	if err != nil {
		t.Fatalf("load reranker: %v", err)
	}
	t.Cleanup(func() { reranker.Close() })
	return reranker
}

func evaluationTopK(t *testing.T) int {
	t.Helper()
	if value := os.Getenv(topKEnv); value != "" {
		topK, err := strconv.Atoi(value)
		if err != nil || topK <= 0 {
			t.Fatalf("%s=%q, expected a positive integer", topKEnv, value)
		}
		return topK
	}
	return config.Defaults().Retrieval.TopK
}

// cachedEmbedder loads the real embedder behind the on-disk vector cache,
// saved when the test ends.
func cachedEmbedder(t *testing.T) *CachingEmbedder {
	t.Helper()
	embedder := loadEmbedder(t)
	cached, err := NewCachingEmbedder(embedder, embeddingCachePath(t, embedder.ContextTokens()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testcheck.NoError(t, cached.Save()) })
	return cached
}

// scaleSizes reads CADE_EVAL_SCALE; none when unset.
func scaleSizes(t *testing.T) []int {
	t.Helper()
	value := os.Getenv(scaleEnv)
	if value == "" {
		return nil
	}
	var sizes []int
	for _, entry := range strings.Split(value, ",") {
		size, err := strconv.Atoi(strings.TrimSpace(entry))
		if err != nil {
			t.Fatalf("%s entry %q, expected an integer", scaleEnv, entry)
		}
		sizes = append(sizes, size)
	}
	return sizes
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
		MaxBestDistance: defaults.Retrieval.MaxBestDistance, QueryPrefix: defaults.Embedding.QueryPrefix, Mode: evalMode(t),
		MaxFilteredEvents: defaults.Retrieval.MaxFilteredEvents}
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

// evalMode lets `MODE=vector go tool mage evalRetrieval` compare modes on the same
// sets; unset means the default.
func evalMode(t *testing.T) rag.Mode {
	t.Helper()
	mode, err := rag.ParseMode(os.Getenv("CADE_EVAL_MODE"))
	if err != nil {
		t.Fatal(err)
	}
	return mode
}

// generationModelEnv points at the answer model; the injection cases need
// it besides the embedder.
const generationModelEnv = "CADE_TEST_GENERATION_MODEL"

// TestInjectionWithModel answers the injection cases with the real models:
// each reply must come from the real evidence, not from the event that
// tells the model what to say.
func TestInjectionWithModel(t *testing.T) {
	warmFixtureCaptions(t)
	embedder, generator := loadEmbedder(t), loadGenerator(t)
	deps := dependencies(t, embedder)
	deps.Settings.MaxAnswerTokens = config.Defaults().Retrieval.MaxAnswerTokens
	corpus, set := loadShippedInjection(t)
	captionCorpus(t, &corpus)
	results, err := RunInjection(context.Background(), deps, generator, corpus, set)
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, result := range results {
		t.Logf("%s\n    evidência: %v\n    %s\n    falhas: %v\n    observações: %v", result.Question, result.Evidence,
			strings.ReplaceAll(result.Reply, "\n", "\n    "), result.Failures, result.Notes)
		failed += min(len(result.Failures), 1)
	}
	if failed > 0 {
		t.Fatalf("%d of %d injection cases failed", failed, len(results))
	}
}

func loadGenerator(t *testing.T) *llamacpp.Generator {
	t.Helper()
	path := os.Getenv(generationModelEnv)
	if path == "" {
		t.Skipf("%s not set; skipping the injection cases", generationModelEnv)
	}
	generation := config.Defaults().Generation
	generator, err := llamacpp.LoadGenerator(llamacpp.ModelOptions{Path: path, ContextTokens: generation.ContextTokens, GPULayers: generation.GPULayers})
	if err != nil {
		t.Fatalf("load generator: %v", err)
	}
	t.Cleanup(func() { generator.Close() })
	return generator
}

// imageFixtureDirectory holds the images the corpus names (phase 19).
const imageFixtureDirectory = "../../testdata/images"

// warmFixtureCaptions describes the corpus images not yet cached, before
// any embedder loads, so the vision model and the embedder never share
// memory; the suite then reads the descriptions from the cache.
func warmFixtureCaptions(t *testing.T) {
	t.Helper()
	if os.Getenv(embeddingModelEnv) == "" {
		t.Skipf("%s not set; skipping model-backed suite", embeddingModelEnv)
	}
	corpus, _ := loadShippedInjection(t)
	captionCorpus(t, &corpus)
}

// captionCorpus fills the image events with their cached (or new)
// descriptions.
func captionCorpus(t *testing.T, corpus *Corpus) {
	t.Helper()
	captions, err := evalimages.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer captions.Close()
	if err := corpus.CaptionImages(context.Background(), imageFixtureDirectory, captions); err != nil {
		t.Fatal(err)
	}
	if err := captions.Save(); err != nil {
		t.Fatal(err)
	}
}

// loadCaptionedSuite is loadShippedSuite with the image events described.
func loadCaptionedSuite(t *testing.T, set string) Suite {
	t.Helper()
	suite := loadShippedSuite(t, set)
	captionCorpus(t, &suite.Corpus)
	return suite
}
