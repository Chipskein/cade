package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

var cliNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

var sampleCommit = event.Event{
	UID: "c1", Source: event.SourceGit, Timestamp: cliNow.Add(-2 * time.Hour), Content: "Fix login",
	Metadata: event.Metadata{"repository": "/src/app", "hash": "abcdef1234567"},
}

// FakeCollector emits a fixed list of events for any target.
type FakeCollector struct {
	Events []event.Event
}

func (f FakeCollector) CollectEvents(_ context.Context, emit ingest.EmitFunc) error {
	for _, ev := range f.Events {
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}

// fakeWorld bundles the fakes behind one Toolkit so tests can inspect them.
type fakeWorld struct {
	store         *testfakes.FakeEventStore
	embedder      *testfakes.FakeEmbedder
	generator     *testfakes.FakeGenerator
	cfg           config.Config
	embedderLoads int
	// generatorLoads counts loads; generatorLoadError fails them.
	generatorLoads     int
	generatorLoadError error
	writtenConfig      string
	writtenCfg         config.Config
	language           Language
	// files, stdin and database back init and doctor.
	files    testfakes.FakeFileSystem
	stdin    string
	database storage.DatabaseState
}

func newFakeWorld() *fakeWorld {
	cfg := config.Defaults()
	cfg.Sources.GitRepositories = []string{"/repo"}
	return &fakeWorld{
		store: testfakes.NewFakeEventStore(), embedder: &testfakes.FakeEmbedder{},
		generator: &testfakes.FakeGenerator{}, cfg: cfg, files: testfakes.NewFakeFileSystem(),
	}
}

func (w *fakeWorld) toolkit() Toolkit {
	return Toolkit{
		DefaultConfigPath: func() (string, error) { return "/cfg/config.json", nil },
		LoadConfig:        func(string) (config.Config, error) { return w.cfg, nil },
		WriteConfig:       w.writeConfig,
		OpenStore:         func(context.Context, string, func(string)) (storage.EventStore, error) { return w.store, nil },
		InspectDatabase:   func(context.Context, string) (storage.DatabaseState, error) { return w.database, nil },
		RootFS:            w.files,
		HomeDir:           func() (string, error) { return "/home/ana", nil },
		Stdin:             strings.NewReader(w.stdin),
		LoadEmbedder: func(config.EmbeddingConfig, *slog.Logger) (ClosableEmbedder, error) {
			w.embedderLoads++
			return w.embedder, nil
		},
		LoadGenerator: w.loadGenerator,
		Sources:       w.sources,
		ReadIndexedDB: w.readIndexedDB,
		Now:           func() time.Time { return cliNow },
		Language:      w.language,
	}
}

func (w *fakeWorld) writeConfig(path string, cfg config.Config) error {
	w.writtenConfig, w.writtenCfg = path, cfg
	return nil
}

func (w *fakeWorld) loadGenerator(config.ModelConfig, *slog.Logger) (ClosableGenerator, error) {
	w.generatorLoads++
	if w.generatorLoadError != nil {
		return nil, w.generatorLoadError
	}
	return w.generator, nil
}

func (w *fakeWorld) sources(cfg config.Config) []ingest.SourceSpec {
	collector := FakeCollector{Events: []event.Event{sampleCommit}}
	newCollector := func(string) (ingest.EventCollector, error) { return collector, nil }
	return []ingest.SourceSpec{
		{Name: "git", DefaultTargets: cfg.Sources.GitRepositories, NewCollector: newCollector},
		{Name: "file", DefaultTargets: cfg.Sources.Directories, NewCollector: newCollector},
	}
}

func (w *fakeWorld) run(args ...string) (int, string, string) {
	var stdout, stderr strings.Builder
	code := Run(context.Background(), args, &stdout, &stderr, w.toolkit())
	return code, stdout.String(), stderr.String()
}

func TestRunWithoutCommandPrintsUsage(t *testing.T) {
	code, _, stderr := newFakeWorld().run()
	if code != 2 || !strings.Contains(stderr, "Uso:") {
		t.Fatalf("expected usage with exit 2, got %d %q", code, stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	code, _, stderr := newFakeWorld().run("dance")
	if code != 2 || !strings.Contains(stderr, `"dance"`) {
		t.Fatalf("expected unknown command error, got %d %q", code, stderr)
	}
}

func TestInitWritesConfigAtGivenPath(t *testing.T) {
	world := newFakeWorld()
	code, stdout, _ := world.run("--config", "/tmp/x.json", "init")
	if code != 0 || world.writtenConfig != "/tmp/x.json" || !strings.Contains(stdout, "/tmp/x.json") {
		t.Fatalf("expected config written to /tmp/x.json, got %d %q %q", code, world.writtenConfig, stdout)
	}
}

func TestIngestUsesConfiguredTargetsAndReports(t *testing.T) {
	world := newFakeWorld()
	code, stdout, stderr := world.run("ingest", "git")
	if code != 0 || len(world.store.Events) != 1 || !strings.Contains(stdout, "git      /repo: 1 novos, 0 atualizados, 0 já existentes") {
		t.Fatalf("expected one ingested commit, got %d %q %q", code, stdout, stderr)
	}
}

func TestIngestAllLoadsEmbedderOnce(t *testing.T) {
	world := newFakeWorld()
	world.cfg.Sources.Directories = []string{"/notes"}
	world.run("ingest", "all")
	if world.embedderLoads != 1 || !world.embedder.Closed || !world.store.Closed {
		t.Fatalf("expected one embedder load and everything closed, got %d loads", world.embedderLoads)
	}
}

func TestIngestWithoutTargetsFailsBeforeLoadingModel(t *testing.T) {
	world := newFakeWorld()
	code, _, stderr := world.run("ingest", "file")
	if code != 1 || world.embedderLoads != 0 || !strings.Contains(stderr, "nenhum alvo") {
		t.Fatalf("expected fast failure, got %d, %d loads, %q", code, world.embedderLoads, stderr)
	}
}

func TestIngestUnknownSource(t *testing.T) {
	code, _, stderr := newFakeWorld().run("ingest", "teams")
	if code != 1 || !strings.Contains(stderr, `"teams"`) {
		t.Fatalf("expected unknown source error, got %d %q", code, stderr)
	}
}

func TestIngestRequiresSource(t *testing.T) {
	if code, _, _ := newFakeWorld().run("ingest"); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestTimelineShowsEventsWithSource(t *testing.T) {
	world := newFakeWorld()
	world.run("ingest", "git")
	code, stdout, _ := world.run("timeline", "hoje")
	if code != 0 || !strings.Contains(stdout, "10:00  [git]     Fix login  (app abcdef12)") {
		t.Fatalf("expected the commit in today's timeline, got %d %q", code, stdout)
	}
}

func TestTimelineEmptyDaySaysSo(t *testing.T) {
	code, stdout, _ := newFakeWorld().run("timeline", "2026-01-01")
	if code != 0 || !strings.Contains(stdout, "Nenhum evento em 2026-01-01") {
		t.Fatalf("expected empty-day message, got %d %q", code, stdout)
	}
}

func TestTimelineRejectsTooManyDates(t *testing.T) {
	if code, _, _ := newFakeWorld().run("timeline", "hoje", "hoje", "hoje"); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestAskPrintsAnswerAndCitedSources(t *testing.T) {
	world := newFakeWorld()
	world.store.SearchResults = []storage.ScoredEvent{{Event: sampleCommit, Distance: 0.1}}
	world.generator.Reply = "Você corrigiu o login [1]."
	code, stdout, _ := world.run("ask", "o que fiz?")
	if code != 0 || !strings.Contains(stdout, "Você corrigiu o login [1].") || !strings.Contains(stdout, "Fontes citadas:\n  [1] [git]") {
		t.Fatalf("expected answer with cited source, got %d %q", code, stdout)
	}
}

func TestAskNotFound(t *testing.T) {
	code, stdout, _ := newFakeWorld().run("ask", "receita de bolo?")
	if code != 0 || !strings.Contains(stdout, "Não encontrei informação") {
		t.Fatalf("expected not-found message, got %d %q", code, stdout)
	}
}

func TestAskPassesFilters(t *testing.T) {
	world := newFakeWorld()
	world.run("ask", "--source", "browser", "--from", "ontem", "o que li?")
	query := world.store.LastQuery
	if query.Source != event.SourceBrowser || !query.From.Equal(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)) || !query.To.Equal(time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected browser filter from yesterday through today, got %+v", query)
	}
}

// Regression: "ontem" in the question used to be matched only by meaning,
// returning a months-old message that contained the word "ontem".
func TestAskResolvesDateInQuestion(t *testing.T) {
	world := newFakeWorld()
	_, _, stderr := world.run("ask", "o que a Ana me passou ontem?")
	query := world.store.LastQuery
	if !query.From.Equal(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)) || !query.To.Equal(time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected yesterday's range, got %s..%s", query.From, query.To)
	}
	if !strings.Contains(stderr, "Entendi: responder · 2026-09-25") {
		t.Fatalf("expected the plan to be announced, got %q", stderr)
	}
}

func TestAskFlagsOverrideDetectedDate(t *testing.T) {
	world := newFakeWorld()
	world.run("ask", "--from", "2026-09-01", "--to", "2026-09-02", "o que fiz ontem?")
	if !world.store.LastQuery.From.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected the flags to win, got %s", world.store.LastQuery.From)
	}
}

func TestAskNoFilters(t *testing.T) {
	world := newFakeWorld()
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Ana"], "direcao": null, "assunto": null}`
	_, _, stderr := world.run("ask", "--no-filters", "o que fiz ontem?")
	if !world.store.LastQuery.From.IsZero() || world.store.LastQuery.Source != "" || !strings.Contains(stderr, "sem filtros") {
		t.Fatalf("expected an unfiltered search, got %+v / %q", world.store.LastQuery, stderr)
	}
}

func TestAskFallsBackWhenPlanIsInvalid(t *testing.T) {
	world := newFakeWorld()
	world.generator.StructuredReply = "{broken"
	code, _, _ := world.run("ask", "o que fiz ontem?")
	if code != 0 || !world.store.LastQuery.From.Equal(time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected an answer with the deterministic date, got %d / %+v", code, world.store.LastQuery)
	}
}

func TestDescribePlan(t *testing.T) {
	query := queryplan.Query{Mode: queryplan.ModeList, Topic: "redis", Source: event.SourceTeams,
		Criteria: listing.Criteria{Direction: listing.Received, People: []string{"Ana"}}}
	if got := describeQuery(query, Portuguese); got != "listar · teams · pessoas: Ana · recebidas · assunto: redis" {
		t.Fatalf("unexpected description %q", got)
	}
}

func TestAskRequiresQuestion(t *testing.T) {
	if code, _, _ := newFakeWorld().run("ask"); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestExitCodeForInterruption(t *testing.T) {
	var stderr strings.Builder
	if code := exitCode(fmt.Errorf("generate: %w", context.Canceled), &stderr, Portuguese); code != 130 || stderr.String() != "interrompido\n" {
		t.Fatalf("expected 130 and interrompido, got %d %q", code, stderr.String())
	}
}

func TestExitCodeReportsError(t *testing.T) {
	var stderr strings.Builder
	if code := exitCode(errors.New("boom"), &stderr, Portuguese); code != 1 || stderr.String() != "erro: boom\n" {
		t.Fatalf("expected exit 1 with message, got %d %q", code, stderr.String())
	}
}

func TestForgetRemovesOnlyThatSource(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{sampleCommit, {UID: "f", Source: event.SourceFile}}
	code, stdout, _ := world.run("forget", "git")
	if code != 0 || len(world.store.Events) != 1 || !strings.Contains(stdout, "1 eventos de git removidos") {
		t.Fatalf("expected only git removed, got %d %q with %d left", code, stdout, len(world.store.Events))
	}
}

func TestForgetRejectsUnknownSource(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{sampleCommit}
	code, _, stderr := world.run("forget", "slack")
	if code != 1 || len(world.store.Events) != 1 || !strings.Contains(stderr, `"slack"`) {
		t.Fatalf("expected rejection without deleting, got %d %q", code, stderr)
	}
}

func TestForgetRequiresOneSource(t *testing.T) {
	if code, _, _ := newFakeWorld().run("forget"); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func teamsEvent(uid string, sentByMe string, kind string) event.Event {
	return event.Event{UID: uid, Source: event.SourceTeams, Timestamp: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
		Content: uid + ": oi", Metadata: event.Metadata{"sent_by_me": sentByMe, "conversation_kind": kind}}
}

func dayOfTeamsMessages(world *fakeWorld) {
	fromAna, fromIanne := teamsEvent("ana-msg", "false", "chat"), teamsEvent("ianne-msg", "false", "chat")
	fromAna.Metadata["sender"], fromIanne.Metadata["sender"] = "Ana Prado - Atlas", "Ianne Rocha - Atlas"
	world.store.Events = []event.Event{fromAna, fromIanne, teamsEvent("enviada", "true", "chat"), sampleCommit}
}

// Regression: "da Ana" was ignored and all 94 received messages were listed.
func TestAskListsFilteredEventsWithoutEmbedder(t *testing.T) {
	world := newFakeWorld()
	dayOfTeamsMessages(world)
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": "ontem", "fonte": "teams", "pessoas": ["Ana"], "direcao": "recebidas", "assunto": null}`
	code, stdout, stderr := world.run("ask", "Me retorne todas as mensagens de ontem que eu recebi da Ana")
	if code != 0 || world.embedderLoads != 0 || !strings.Contains(stderr, "Filtrando por pessoa: ana") {
		t.Fatalf("expected a person-filtered listing without the embedder, got %d, %d loads, %q", code, world.embedderLoads, stderr)
	}
	if !strings.Contains(stdout, "ana-msg") || strings.Contains(stdout, "ianne-msg") || strings.Contains(stdout, "enviada") || strings.Contains(stdout, "Fix login") {
		t.Fatalf("expected only Ana's message, got:\n%s", stdout)
	}
}

// A listing the rules read needs neither model.
func TestAskListReadByRulesLoadsNoModel(t *testing.T) {
	world := newFakeWorld()
	dayOfTeamsMessages(world)
	code, stdout, stderr := world.run("ask", "liste as mensagens de ontem")
	if code != 0 || world.generatorLoads != 0 || world.embedderLoads != 0 || !strings.Contains(stdout, "ana-msg") {
		t.Fatalf("expected a listing without models, got %d, %d/%d loads, %q %q", code, world.generatorLoads, world.embedderLoads, stdout, stderr)
	}
}

func TestAskStopsWhenGeneratorDoesNotLoad(t *testing.T) {
	world := newFakeWorld()
	world.generatorLoadError = errors.New("model file missing")
	code, _, stderr := world.run("ask", "o que a Ana me pediu?")
	if code != 1 || !strings.Contains(stderr, "model file missing") {
		t.Fatalf("expected the load error, got %d %q", code, stderr)
	}
}

func TestAskListingWithoutPeriodAnswersInstead(t *testing.T) {
	world := newFakeWorld()
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": null, "fonte": "teams", "pessoas": [], "direcao": null, "assunto": null}`
	world.run("ask", "as mensagens sobre deploy")
	if world.embedderLoads != 1 || world.store.LastQuery.Source != event.SourceTeams {
		t.Fatalf("expected a semantic search restricted to teams, got %d loads / %+v", world.embedderLoads, world.store.LastQuery)
	}
}

// An unknown name ("CEP" read as a person) filters as text instead of
// being dropped, which listed the whole day.
func TestAskUnknownPersonFiltersListingAsText(t *testing.T) {
	world := newFakeWorld()
	dayOfTeamsMessages(world)
	world.generator.StructuredReply = `{"tipo": "listar", "periodo": "ontem", "fonte": null, "pessoas": ["CEP"], "direcao": null, "assunto": null}`
	_, stdout, stderr := world.run("ask", "tudo sobre o CEP ontem")
	if !strings.Contains(stderr, "buscando como texto: CEP") || strings.Contains(stdout, "ana-msg") {
		t.Fatalf("expected CEP searched as text, got %q / %q", stderr, stdout)
	}
}
