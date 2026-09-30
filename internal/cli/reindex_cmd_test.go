package cli

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

const configuredModel = "nomic-embed-text-v2-moe.Q4_K_M.gguf"

func TestReindexRecomputesVectorsWithConfiguredModel(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{sampleCommit, {UID: "c2", Source: event.SourceGit, Timestamp: cliNow, Content: "Outro commit"}}
	world.store.EmbeddingModelName = "nomic-embed-text-v1.5.Q8_0.gguf"
	code, stdout, stderr := world.run("reindex")
	if code != 0 || !strings.Contains(stdout, "2 eventos reindexados com "+configuredModel) || world.store.EmbeddingModelName != configuredModel || world.store.Pending {
		t.Fatalf("expected both events reindexed with the configured model, got %d %q %q", code, stdout, stderr)
	}
}

const previousModel = "nomic-embed-text-v1.5.Q8_0.gguf"

// The gates were set for the previous model and left as they were.
func TestReindexWarnsThatGatesBelongToThePreviousModel(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{sampleCommit}
	world.store.EmbeddingModelName = previousModel
	code, _, stderr := world.run("reindex")
	if code != 0 || !strings.Contains(stderr, "calibrados para "+previousModel+" e não valem para "+configuredModel) ||
		!strings.Contains(stderr, "go tool mage evalRetrieval") || world.store.Calibration.Model != previousModel {
		t.Fatalf("expected a warning and the calibration kept for %s, got %d %q %+v", previousModel, code, stderr, world.store.Calibration)
	}
}

// Changing a gate after the model change counts as recalibrating.
func TestReindexAcceptsGatesRetunedForTheNewModel(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{sampleCommit}
	world.store.EmbeddingModelName = previousModel
	world.store.Calibration = storage.ThresholdCalibration{Model: previousModel, MaxDistance: 0.5, MaxBestDistance: 0.4}
	code, _, stderr := world.run("reindex")
	if code != 0 || strings.Contains(stderr, "calibrados") || world.store.Calibration.Model != configuredModel {
		t.Fatalf("expected no warning and the new model recorded, got %d %q %+v", code, stderr, world.store.Calibration)
	}
}

func TestIngestRecordsTheGatesInEffect(t *testing.T) {
	world := newFakeWorld()
	defaults := config.Defaults().Retrieval
	want := storage.ThresholdCalibration{Model: configuredModel, MaxDistance: defaults.MaxDistance, MaxBestDistance: defaults.MaxBestDistance}
	if code, _, _ := world.run("ingest", "git"); code != 0 || world.store.Calibration != want {
		t.Fatalf("expected %+v recorded, got %d %+v", want, code, world.store.Calibration)
	}
}

// Regression guard: models of the same dimension were mixed silently.
func TestIngestRefusesVectorsOfAnotherModel(t *testing.T) {
	world := newFakeWorld()
	world.store.EmbeddingModelName = "nomic-embed-text-v1.5.Q8_0.gguf"
	code, _, stderr := world.run("ingest", "git")
	if code != 1 || !strings.Contains(stderr, `indexado com "nomic-embed-text-v1.5.Q8_0.gguf"`) || !strings.Contains(stderr, "cade reindex") || len(world.store.Events) != 0 {
		t.Fatalf("expected the mismatch refused before ingesting, got %d %q", code, stderr)
	}
}

func TestAskAnswerRefusesVectorsOfAnotherModel(t *testing.T) {
	world := newFakeWorld()
	world.store.EmbeddingModelName = "outro.gguf"
	world.store.SearchResults = []storage.ScoredEvent{{Event: sampleCommit, Distance: 0.1}}
	if code, _, stderr := world.run("ask", "o que fiz?"); code != 1 || !strings.Contains(stderr, "cade reindex") {
		t.Fatalf("expected the answer refused, got %d %q", code, stderr)
	}
}

// A database from before the model was recorded adopts the configured one.
func TestIngestRecordsModelOnLegacyDatabase(t *testing.T) {
	world := newFakeWorld()
	if code, _, _ := world.run("ingest", "git"); code != 0 || world.store.EmbeddingModelName != configuredModel {
		t.Fatalf("expected the configured model recorded, got %d %q", code, world.store.EmbeddingModelName)
	}
}

func TestPendingReindexIsWarned(t *testing.T) {
	world := newFakeWorld()
	world.store.EmbeddingModelName, world.store.Pending = configuredModel, true
	if _, _, stderr := world.run("ingest", "git"); !strings.Contains(stderr, "Reindexação incompleta") {
		t.Fatalf("expected the pending rebuild warned, got %q", stderr)
	}
}

func TestReindexRejectsArguments(t *testing.T) {
	if code, _, stderr := newFakeWorld().run("reindex", "git"); code != 1 || !strings.Contains(stderr, `não recebe argumentos`) {
		t.Fatalf("expected a usage error, got %d %q", code, stderr)
	}
}

func TestReindexProgressLogsEveryTenPercentOffTerminal(t *testing.T) {
	var out strings.Builder
	progress := reindexProgress(&statusLine{out: &out}, Portuguese)
	for done := 1; done <= 100; done++ {
		progress(done, 100)
	}
	if lines := strings.Count(out.String(), "\n"); lines != 11 {
		t.Fatalf("expected one line per decile, got %d:\n%s", lines, out.String())
	}
}
