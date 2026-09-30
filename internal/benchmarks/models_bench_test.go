package benchmarks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

// The model paths come from the same variables as the model tests
// (mage targets bench and testModels); without them these skip.
const (
	embeddingModelEnv  = "CADE_TEST_EMBEDDING_MODEL"
	generationModelEnv = "CADE_TEST_GENERATION_MODEL"
)

// sampleMessage has the length of an average stored Teams message.
const sampleMessage = "Carla Dias: esse CEP que está cadastrado não existe mais, precisa atualizar antes de sincronizar\nConversa: chat Carla Dias, Eu\nRecebida por você"

func modelPath(b *testing.B, variable string) string {
	b.Helper()
	path := os.Getenv(variable)
	if path == "" {
		b.Skipf("%s not set; skipping model benchmark", variable)
	}
	return path
}

// The first call on a GPU pays for CUDA setup (it made a 5-iteration run
// report 27 ms per embedding instead of ~3 ms), so every model benchmark
// warms up before b.Loop, which excludes setup from the timing.

func loadEmbedder(b *testing.B) *llamacpp.Embedder {
	b.Helper()
	embedder, err := llamacpp.LoadEmbedder(llamacpp.ModelOptions{Path: modelPath(b, embeddingModelEnv), GPULayers: -1})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { embedder.Close() })
	return embedder
}

func loadGenerator(b *testing.B) *llamacpp.Generator {
	b.Helper()
	defaults := config.Defaults().Generation
	generator, err := llamacpp.LoadGenerator(llamacpp.ModelOptions{Path: modelPath(b, generationModelEnv), ContextTokens: defaults.ContextTokens, GPULayers: -1})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { generator.Close() })
	return generator
}

// BenchmarkEmbedEvent is ingestion's cost per event with text.
func BenchmarkEmbedEvent(b *testing.B) {
	embedder := loadEmbedder(b)
	prefix := config.Defaults().Embedding.DocumentPrefix
	if _, err := embedder.Embed(prefix + sampleMessage); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := embedder.Embed(prefix + sampleMessage); err != nil {
			b.Fatal(err)
		}
	}
	reportMemory(b)
}

// planQuestions all name a person or a topic, so the model reads them;
// questions the rules read skip it (see BenchmarkColdAsk).
var planQuestions = []string{"o que a Carla me pediu ontem?", "liste os commits de 20/09 sobre autenticação", "quais tarefas o Rui me passou essa semana?"}

// BenchmarkPlanQuestion measures the model interpreting one question with a
// cold prompt cache and no saved prompt state, as each `cade ask` process
// did before the state was saved.
func BenchmarkPlanQuestion(b *testing.B) {
	generator := loadGenerator(b)
	planner := queryplan.NewPlanner(generator)
	if _, err := planner.Plan(context.Background(), planQuestions[0]); err != nil {
		b.Fatal(err)
	}
	i := 0
	for b.Loop() {
		b.StopTimer()
		evictPromptCache(b, generator)
		b.StartTimer()
		if _, err := planner.Plan(context.Background(), planQuestions[i%len(planQuestions)]); err != nil {
			b.Fatal(err)
		}
		i++
	}
	reportMemory(b)
}

// evictPromptCache runs an unrelated prompt so the next one starts cold.
func evictPromptCache(b *testing.B, generator llm.StructuredGenerator) {
	b.Helper()
	messages := []llm.ChatMessage{{Role: llm.RoleUser, Content: fmt.Sprintf("diga ok %d", time.Now().UnixNano())}}
	if _, err := generator.GenerateStructured(context.Background(), messages, 2, `root ::= "ok"`); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkAnswer generates a reply from top_k evidence events of the
// maximum evidence length, the slowest part of `cade ask`.
func BenchmarkAnswer(b *testing.B) {
	generator := loadGenerator(b)
	settings := config.Defaults().Retrieval
	var tokens int
	answerer := rag.NewAnswerer(rag.Dependencies{Store: evidenceStore(settings.TopK), Embedder: &testfakes.FakeEmbedder{Vector: []float32{1}},
		Generator: generator, Now: time.Now, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))},
		rag.Settings{TopK: settings.TopK, MaxDistance: settings.MaxDistance, MaxBestDistance: settings.MaxBestDistance, MaxAnswerTokens: settings.MaxAnswerTokens})
	if _, err := answerer.Answer(context.Background(), queryplan.Query{Question: "aquecimento", Source: event.SourceTeams}, rag.AnswerObserver{}); err != nil {
		b.Fatal(err)
	}
	observer := rag.AnswerObserver{Generation: llm.GenerationProgress{TokenGenerated: func(string) { tokens++ }}}
	i := 0
	for b.Loop() {
		query := queryplan.Query{Question: fmt.Sprintf("qual o problema com o CEP? (%d)", i), Source: event.SourceTeams}
		if _, err := answerer.Answer(context.Background(), query, observer); err != nil {
			b.Fatal(err)
		}
		i++
	}
	b.ReportMetric(float64(tokens)/float64(b.N), "reply_tokens/op")
	reportMemory(b)
}

// evidenceStore returns count Teams messages cut at the prompt's 700
// characters, as the fake store's search results.
func evidenceStore(count int) *testfakes.FakeEventStore {
	store := testfakes.NewFakeEventStore()
	for i := range count {
		content := strings.Repeat(fmt.Sprintf("mensagem %d sobre o CEP da transportadora e a sincronização; ", i), 12)[:700]
		store.SearchResults = append(store.SearchResults, storage.ScoredEvent{Distance: 0.3,
			Event: event.Event{UID: strconv.Itoa(i), Source: event.SourceTeams, Timestamp: time.Now().Add(-time.Hour), Content: content}})
	}
	return store
}

// reportMemory adds the process's resident memory and, on a CUDA build,
// its GPU memory, read from /proc and nvidia-smi.
func reportMemory(b *testing.B) {
	b.Helper()
	if rss, ok := residentMegabytes(); ok {
		b.ReportMetric(rss, "rss_MB")
	}
	if gpu, ok := gpuMegabytes(); ok {
		b.ReportMetric(gpu, "gpu_MB")
	}
}

func residentMegabytes() (float64, bool) {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(status), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "VmRSS:" {
			kilobytes, err := strconv.ParseFloat(fields[1], 64)
			return kilobytes / 1024, err == nil
		}
	}
	return 0, false
}

func gpuMegabytes() (float64, bool) {
	output, err := exec.Command("nvidia-smi", "--query-compute-apps=pid,used_memory", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0, false
	}
	pid := strconv.Itoa(os.Getpid())
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, ",")
		if len(fields) == 2 && strings.TrimSpace(fields[0]) == pid {
			megabytes, err := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
			return megabytes, err == nil
		}
	}
	return 0, false
}
