package benchmarks

import (
	"context"
	"io"
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/llm/llamacpp"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
)

// Questions of BenchmarkColdAsk: the first needs the model to be read (a
// topic), the second is read by rules.
const (
	modelReadQuestion = "qual o problema com o CEP da transportadora?"
	rulesReadQuestion = "o que eu fiz hoje?"
)

// coldAskPlanner is how the question gets read in one variant.
type coldAskPlanner struct {
	name       string
	question   string
	savedState bool
}

var coldAskPlanners = []coldAskPlanner{
	{name: "model", question: modelReadQuestion},
	{name: "model+state", question: modelReadQuestion, savedState: true},
	{name: "rules", question: rulesReadQuestion},
}

// BenchmarkColdAsk is what one `cade ask` costs from the start of the
// process to the first answer token: loading both models, reading the
// question, embedding it and reading the evidence. The page cache is warm
// (models read recently) or cold (evicted before each run, as after a
// reboot); the planner decodes its prompt, loads the saved prompt state, or
// is skipped by the rules. Opening the database is left out: it is measured
// in the sqlitestore benchmarks.
func BenchmarkColdAsk(b *testing.B) {
	paths := coldAskPaths{generation: modelPath(b, generationModelEnv), embedding: modelPath(b, embeddingModelEnv)}
	for _, page := range []string{"warm", "cold"} {
		for _, planner := range coldAskPlanners {
			b.Run("page="+page+"/planner="+planner.name, func(b *testing.B) {
				benchmarkColdAsk(b, paths, planner, page == "cold")
			})
		}
	}
}

type coldAskPaths struct {
	generation, embedding string
}

// benchmarkColdAsk warms up once (CUDA setup, and the saved state is
// written), so each timed run is a later `cade ask`.
func benchmarkColdAsk(b *testing.B, paths coldAskPaths, planner coldAskPlanner, evict bool) {
	stateDir := ""
	if planner.savedState {
		stateDir = b.TempDir()
	}
	askToFirstToken(b, paths, planner.question, stateDir)
	for b.Loop() {
		if evict {
			b.StopTimer()
			evictFromPageCache(b, paths.generation, paths.embedding)
			b.StartTimer()
		}
		askToFirstToken(b, paths, planner.question, stateDir)
	}
}

// askToFirstToken runs `cade ask`'s model work, generating one token.
func askToFirstToken(b *testing.B, paths coldAskPaths, question, stateDir string) {
	b.Helper()
	defaults := config.Defaults()
	generator, err := llamacpp.LoadGenerator(llamacpp.ModelOptions{Path: paths.generation, ContextTokens: defaults.Generation.ContextTokens,
		GPULayers: -1, PromptStateDir: stateDir})
	if err != nil {
		b.Fatal(err)
	}
	defer generator.Close()
	plan, err := queryplan.NewPlanner(generator).Plan(context.Background(), question)
	if err != nil {
		b.Fatal(err)
	}
	embedder, err := llamacpp.LoadEmbedder(llamacpp.ModelOptions{Path: paths.embedding, GPULayers: -1})
	if err != nil {
		b.Fatal(err)
	}
	defer embedder.Close()
	answerFirstToken(b, generator, embedder, queryplan.Resolve(question, plan, queryplan.Overrides{}, time.Now()))
}

func answerFirstToken(b *testing.B, generator llm.Generator, embedder llm.Embedder, query queryplan.Query) {
	b.Helper()
	settings := config.Defaults().Retrieval
	store := evidenceStore(settings.TopK)
	store.Events = eventsOf(store.SearchResults)
	answerer := rag.NewAnswerer(rag.Dependencies{Store: store, Embedder: embedder, Generator: generator, Now: time.Now,
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))},
		rag.Settings{TopK: settings.TopK, MaxDistance: 2, MaxBestDistance: 2, MaxAnswerTokens: 1})
	tokens := 0
	observer := rag.AnswerObserver{Generation: llm.GenerationProgress{TokenGenerated: func(string) { tokens++ }}}
	if _, err := answerer.Answer(context.Background(), query, observer); err != nil {
		b.Fatal(err)
	}
	if tokens == 0 {
		b.Fatalf("no answer token for %q: the evidence did not reach the generator", query.Question)
	}
}

// posixFadvDontNeed is POSIX_FADV_DONTNEED on Linux.
const posixFadvDontNeed = 4

// evictFromPageCache drops paths from the page cache without root, so the
// next load reads them from disk as after a reboot. The models must not
// be mapped by anyone at that moment (they are closed after each run).
func evictFromPageCache(b *testing.B, paths ...string) {
	b.Helper()
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		_, _, errno := syscall.Syscall6(syscall.SYS_FADVISE64, file.Fd(), 0, 0, posixFadvDontNeed, 0, 0)
		file.Close()
		if errno != 0 {
			b.Fatalf("evict %q from the page cache: %v", path, errno)
		}
	}
}

// eventsOf lists the events of hits, so a question with a period finds
// them through EventsBetween as one without finds them by search.
func eventsOf(hits []storage.ScoredEvent) []event.Event {
	events := make([]event.Event, len(hits))
	for i, hit := range hits {
		events[i] = hit.Event
	}
	return events
}
