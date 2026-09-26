package llamacpp

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/llm"
)

// Model-backed tests need real GGUF files, so they run only when these
// variables point at them (see Makefile target test-models).
const (
	embeddingModelEnv  = "CADE_TEST_EMBEDDING_MODEL"
	generationModelEnv = "CADE_TEST_GENERATION_MODEL"
)

func modelPathOrSkip(t *testing.T, variable string) string {
	t.Helper()
	path := os.Getenv(variable)
	if path == "" {
		t.Skipf("%s not set; skipping model-backed test", variable)
	}
	return path
}

func cosineSimilarity(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func TestNormalizeProducesUnitLength(t *testing.T) {
	normalized := normalize([]float32{3, 4})
	if math.Abs(float64(normalized[0])-0.6) > 1e-6 || math.Abs(float64(normalized[1])-0.8) > 1e-6 {
		t.Fatalf("expected [0.6 0.8], got %v", normalized)
	}
}

func TestNormalizeKeepsZeroVector(t *testing.T) {
	if normalized := normalize([]float32{0, 0}); normalized[0] != 0 || normalized[1] != 0 {
		t.Fatalf("expected zero vector, got %v", normalized)
	}
}

func TestTotalContentLength(t *testing.T) {
	messages := []llm.ChatMessage{{Role: llm.RoleUser, Content: "abc"}}
	if got := totalContentLength(messages); got != len("user")+3 {
		t.Fatalf("expected 7, got %d", got)
	}
}

func TestEmbedderRanksRelatedTextCloser(t *testing.T) {
	embedder, err := LoadEmbedder(ModelOptions{Path: modelPathOrSkip(t, embeddingModelEnv)})
	if err != nil {
		t.Fatalf("load embedder: %v", err)
	}
	defer embedder.Close()
	query, _ := embedder.Embed("search_query: fixing a database migration")
	related, _ := embedder.Embed("search_document: fix broken SQL migration for users table")
	unrelated, _ := embedder.Embed("search_document: recipe for chocolate cake")
	if cosineSimilarity(query, related) <= cosineSimilarity(query, unrelated) {
		t.Fatal("expected the migration commit to be closer to the query than the recipe")
	}
}

func TestEmbedderRejectsMissingModel(t *testing.T) {
	if _, err := LoadEmbedder(ModelOptions{Path: "/nonexistent/model.gguf"}); err == nil {
		t.Fatal("expected an error for a missing model file")
	}
}

func loadTestGenerator(t *testing.T, contextTokens int) *Generator {
	t.Helper()
	generator, err := LoadGenerator(ModelOptions{Path: modelPathOrSkip(t, generationModelEnv), ContextTokens: contextTokens})
	if err != nil {
		t.Fatalf("load generator: %v", err)
	}
	t.Cleanup(func() { generator.Close() })
	return generator
}

var capitalQuestion = []llm.ChatMessage{
	{Role: llm.RoleSystem, Content: "Reply with exactly one word."},
	{Role: llm.RoleUser, Content: "What is the capital of France?"},
}

func TestGeneratorFollowsInstruction(t *testing.T) {
	reply, err := loadTestGenerator(t, 2048).Generate(context.Background(), capitalQuestion, 16, llm.GenerationProgress{})
	if err != nil || !strings.Contains(strings.ToLower(reply), "paris") {
		t.Fatalf("expected a reply mentioning Paris, got %q (err %v)", reply, err)
	}
}

func TestGeneratorStreamsTokensAndPromptProgress(t *testing.T) {
	var streamed string
	var lastDone, lastTotal int
	progress := llm.GenerationProgress{
		PromptProcessed: func(done, total int) { lastDone, lastTotal = done, total },
		TokenGenerated:  func(piece string) { streamed += piece },
	}
	reply, err := loadTestGenerator(t, 2048).Generate(context.Background(), capitalQuestion, 16, progress)
	if err != nil || streamed != reply || lastTotal == 0 || lastDone != lastTotal {
		t.Fatalf("expected streamed == reply and full prompt progress, got %q vs %q, %d/%d (err %v)", streamed, reply, lastDone, lastTotal, err)
	}
}

func TestGeneratorStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadTestGenerator(t, 2048).Generate(ctx, capitalQuestion, 16, llm.GenerationProgress{}); err == nil {
		t.Fatal("expected a cancellation error")
	}
}

func TestGeneratorRejectsPromptLargerThanContext(t *testing.T) {
	long := strings.Repeat("activity log entry ", 400)
	_, err := loadTestGenerator(t, 256).Generate(context.Background(), []llm.ChatMessage{{Role: llm.RoleUser, Content: long}}, 16, llm.GenerationProgress{})
	if err == nil {
		t.Fatal("expected an error when the prompt exceeds the context")
	}
}

func TestThreadCount(t *testing.T) {
	if threadCount(3, 12) != 3 || threadCount(0, 12) != 6 || threadCount(0, 1) != 1 {
		t.Fatal("expected configured value, else half the logical CPUs, at least 1")
	}
}

func TestUTF8StreamerHoldsPartialCharacters(t *testing.T) {
	var pieces []string
	stream := utf8Streamer{emit: func(piece string) { pieces = append(pieces, piece) }}
	emoji := []byte("🎉")
	stream.write([]byte("ok "))
	stream.write(emoji[:2])
	stream.write(emoji[2:])
	if len(pieces) != 2 || pieces[1] != "🎉" || stream.finish() != "ok 🎉" {
		t.Fatalf("expected [ok  🎉], got %q / %q", pieces, stream.finish())
	}
}

func TestGenerateStructuredFollowsGrammar(t *testing.T) {
	grammar := `root ::= "{\"cor\": \"" ("azul" | "verde") "\"}"`
	messages := []llm.ChatMessage{{Role: llm.RoleUser, Content: "Qual a cor do céu? Responda em JSON."}}
	reply, err := loadTestGenerator(t, 1024).GenerateStructured(context.Background(), messages, 16, grammar)
	if err != nil || (reply != `{"cor": "azul"}` && reply != `{"cor": "verde"}`) {
		t.Fatalf("expected grammar-conforming JSON, got %q (err %v)", reply, err)
	}
}

func TestGenerateStructuredRejectsInvalidGrammar(t *testing.T) {
	if _, err := loadTestGenerator(t, 1024).GenerateStructured(context.Background(), capitalQuestion, 8, "nonsense ::="); err == nil {
		t.Fatal("expected an error for an invalid grammar")
	}
}

func TestCommonPrefix(t *testing.T) {
	if commonPrefix([]int{1, 2, 3}, []int{1, 2, 4}) != 2 || commonPrefix([]int{1}, []int{1, 2}) != 1 || commonPrefix(nil, []int{1}) != 0 {
		t.Fatal("unexpected common prefix")
	}
}

// Reusing the cached prefix must not change what the model answers.
func TestStructuredReplyIsStableAcrossCachedPrompts(t *testing.T) {
	generator := loadTestGenerator(t, 2048)
	grammar := `root ::= "sim" | "nao"`
	ask := func(question string) string {
		messages := []llm.ChatMessage{{Role: llm.RoleSystem, Content: "Responda sim ou nao."}, {Role: llm.RoleUser, Content: question}}
		reply, err := generator.GenerateStructured(context.Background(), messages, 4, grammar)
		if err != nil {
			t.Fatal(err)
		}
		return reply
	}
	first := ask("O céu é azul?")
	ask("Peixes voam?")
	if again := ask("O céu é azul?"); again != first {
		t.Fatalf("expected the same reply with a cached prefix, got %q then %q", first, again)
	}
}
