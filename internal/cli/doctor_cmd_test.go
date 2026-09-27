package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/doctor"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

// doctorWorld is an installation where everything the config names
// exists: a config file, both models, one repository and a database.
func doctorWorld() *fakeWorld {
	world := newFakeWorld()
	world.cfg.Embedding.ModelPath, world.cfg.Generation.ModelPath = "/home/ana/models/nomic.gguf", "/home/ana/models/qwen.gguf"
	world.cfg.DatabasePath = "/home/ana/cade.db"
	world.files = testfakes.NewFakeFileSystem().
		AddFile("/cfg/config.json", "{}").
		AddFile("/home/ana/models/nomic.gguf", "GGUF...").
		AddFile("/home/ana/models/qwen.gguf", "GGUF...").
		AddDir("/repo/.git")
	world.database = storage.DatabaseState{Exists: true, FTS5: true, SchemaVersion: 7, LatestSchemaVersion: 7, EmbeddingModel: "nomic.gguf"}
	return world
}

func TestDoctorHealthyInstall(t *testing.T) {
	code, stdout, stderr := doctorWorld().run("doctor")
	if code != 0 || !strings.Contains(stdout, "Tudo pronto (0 avisos)") || strings.Contains(stdout, "falha") {
		t.Fatalf("expected a clean report, got %d %q %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "ok     modelo de geração      ~/models/qwen.gguf\n") || !strings.Contains(stdout, "busca por palavras     SQLite FTS5\n") {
		t.Fatalf("expected aligned rows with ~ paths, got %q", stdout)
	}
}

func TestDoctorFailsOnMissingModel(t *testing.T) {
	world := doctorWorld()
	world.cfg.Generation.ModelPath = "/home/ana/models/absent.gguf"
	code, stdout, stderr := world.run("doctor")
	if code != 1 || !strings.Contains(stdout, "rode `make models` ou ajuste `generation.model_path`") || !strings.Contains(stderr, "1 problema a corrigir") {
		t.Fatalf("expected the missing model and exit 1, got %d %q %q", code, stdout, stderr)
	}
}

func TestDoctorWarningsKeepExitZero(t *testing.T) {
	world := doctorWorld()
	world.database = storage.DatabaseState{Exists: true, FTS5: true, SchemaVersion: 4, LatestSchemaVersion: 7, MigrationBackup: true, SizeBytes: 450 << 20}
	code, stdout, _ := world.run("doctor")
	if code != 0 || !strings.Contains(stdout, "esquema v4; o próximo comando migra para v7, antes gravando uma cópia (~450 MB)") {
		t.Fatalf("expected a migration warning with exit 0, got %d %q", code, stdout)
	}
}

func TestDoctorInEnglish(t *testing.T) {
	world := doctorWorld()
	world.language = English
	world.files.AddDir("/plain")
	world.cfg.Sources.GitRepositories = []string{"/plain"}
	code, stdout, stderr := world.run("doctor")
	if code != 1 || !strings.Contains(stdout, "fail   source git") || !strings.Contains(stdout, "is not a git repository") || !strings.Contains(stderr, "1 problem to fix") {
		t.Fatalf("expected an English report, got %d %q %q", code, stdout, stderr)
	}
}

func TestDoctorStopsOnUnreadableConfig(t *testing.T) {
	world := doctorWorld()
	world.cfg = config.Config{}
	toolkit := world.toolkit()
	toolkit.LoadConfig = func(string) (config.Config, error) { return config.Config{}, errors.New("parse config: bad JSON") }
	var stdout, stderr strings.Builder
	if code := Run(t.Context(), []string{"doctor"}, &stdout, &stderr, toolkit); code != 1 || !strings.Contains(stderr.String(), "bad JSON") {
		t.Fatalf("expected the parse error, got %d %q", code, stderr.String())
	}
}

// Every problem has wording in both languages, without a formatting slip.
func TestEveryProblemIsWorded(t *testing.T) {
	for problem := doctor.ProblemNoConfigFile; problem <= doctor.ProblemReindexPending; problem++ {
		finding := doctor.Finding{Problem: problem, Setting: "sources.directories"}
		for _, language := range []Language{Portuguese, English} {
			text := problemText(finding, language)
			if strings.HasPrefix(text, "problem ") || strings.Contains(text, "%!") {
				t.Errorf("problem %d in language %d is worded %q", problem, language, text)
			}
		}
	}
}
