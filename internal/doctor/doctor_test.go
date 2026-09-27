package doctor

import (
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

const (
	configPath     = "/home/ana/.config/cade/config.json"
	embeddingPath  = "/models/nomic.gguf"
	generationPath = "/models/qwen.gguf"
)

// healthyInstall is a machine where everything the config names exists.
func healthyInstall() testfakes.FakeFileSystem {
	return testfakes.NewFakeFileSystem().
		AddFile(configPath, "{}").
		AddFile(embeddingPath, "GGUF\x03rest").
		AddFile(generationPath, "GGUF\x03rest").
		AddDir("/src/api/.git").
		AddFile("/chrome/History", "SQLite format 3\x00tables").
		AddDir("/notes").
		AddFile("/chrome/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb/CURRENT", "MANIFEST-000001\n")
}

func healthyConfig() config.Config {
	cfg := config.Defaults()
	cfg.Embedding.ModelPath, cfg.Generation.ModelPath = embeddingPath, generationPath
	cfg.Sources.GitRepositories = []string{"/src/api"}
	cfg.Sources.BrowserHistories = []string{"/chrome/History"}
	cfg.Sources.Directories = []string{"/notes"}
	cfg.Sources.TeamsIndexedDBDirs = []string{"/chrome/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb"}
	return cfg
}

func currentDatabase() storage.DatabaseState {
	return storage.DatabaseState{Exists: true, FTS5: true, SchemaVersion: 7, LatestSchemaVersion: 7, EmbeddingModel: "nomic.gguf"}
}

func findingFor(t *testing.T, findings []Finding, subject Subject, path string) Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.Subject == subject && finding.Path == path {
			return finding
		}
	}
	t.Fatalf("no finding for subject %d path %q in %+v", subject, path, findings)
	return Finding{}
}

func TestHealthyInstallHasNoProblems(t *testing.T) {
	findings := Diagnose(healthyInstall(), configPath, healthyConfig(), currentDatabase())
	if len(findings) != 9 || CountBySeverity(findings, SeverityOK) != 9 {
		t.Fatalf("expected 9 clean findings, got %+v", findings)
	}
}

func TestMissingConfigFileAndSourcesAreWarnings(t *testing.T) {
	cfg := healthyConfig()
	cfg.Sources = config.Defaults().Sources
	findings := Diagnose(healthyInstall(), "/elsewhere/config.json", cfg, currentDatabase())
	if findingFor(t, findings, SubjectConfigFile, "/elsewhere/config.json").Problem != ProblemNoConfigFile {
		t.Fatalf("expected the missing config file reported, got %+v", findings)
	}
	if findingFor(t, findings, SubjectSource, "").Problem != ProblemNoSources || CountBySeverity(findings, SeverityFailure) != 0 {
		t.Fatalf("expected only warnings, got %+v", findings)
	}
}

func TestModelProblems(t *testing.T) {
	fsys := healthyInstall().AddFile(generationPath, "<html>not found</html>")
	cfg := healthyConfig()
	cfg.Embedding.ModelPath = "/models/absent.gguf"
	findings := Diagnose(fsys, configPath, cfg, currentDatabase())
	if got := findingFor(t, findings, SubjectEmbeddingModel, "/models/absent.gguf"); got.Problem != ProblemMissing || got.Setting != "embedding.model_path" {
		t.Fatalf("expected the missing embedding model, got %+v", got)
	}
	if got := findingFor(t, findings, SubjectGenerationModel, generationPath); got.Problem != ProblemNotGGUF {
		t.Fatalf("expected a non-GGUF generation model, got %+v", got)
	}
}

func TestSourceProblems(t *testing.T) {
	fsys := healthyInstall().AddDir("/src/plain").AddFile("/chrome/Bookmarks", "{}").AddFile("/notes.md", "").AddDir("/empty-idb")
	cfg := healthyConfig()
	cfg.Sources.GitRepositories = []string{"/src/plain", "/src/absent"}
	cfg.Sources.BrowserHistories = []string{"/chrome/Bookmarks", "/chrome"}
	cfg.Sources.Directories = []string{"/notes.md"}
	cfg.Sources.TeamsIndexedDBDirs = []string{"/empty-idb"}
	findings := Diagnose(fsys, configPath, cfg, currentDatabase())
	want := map[string]Problem{
		"/src/plain": ProblemNotGitRepository, "/src/absent": ProblemMissing, "/chrome/Bookmarks": ProblemNotSQLite,
		"/chrome": ProblemNotAFile, "/notes.md": ProblemNotADirectory, "/empty-idb": ProblemNotLevelDB,
	}
	for path, problem := range want {
		if got := findingFor(t, findings, SubjectSource, path); got.Problem != problem {
			t.Errorf("%s: expected problem %d, got %+v", path, problem, got)
		}
	}
}

func TestKeywordSearchWithoutFTS5(t *testing.T) {
	database := currentDatabase()
	database.FTS5 = false
	findings := Diagnose(healthyInstall(), configPath, healthyConfig(), database)
	if findingFor(t, findings, SubjectKeywordSearch, "").Problem != ProblemNoFTS5 {
		t.Fatalf("expected FTS5 reported, got %+v", findings)
	}
}

func TestDatabaseProblems(t *testing.T) {
	cases := map[string]struct {
		change func(*storage.DatabaseState)
		want   Problem
	}{
		"missing":     {func(d *storage.DatabaseState) { *d = storage.DatabaseState{FTS5: true, LatestSchemaVersion: 7} }, ProblemNoDatabase},
		"too new":     {func(d *storage.DatabaseState) { d.SchemaVersion = 9 }, ProblemSchemaTooNew},
		"other model": {func(d *storage.DatabaseState) { d.EmbeddingModel = "bge.gguf" }, ProblemEmbeddingMismatch},
		"reindex":     {func(d *storage.DatabaseState) { d.ReindexPending = true }, ProblemReindexPending},
		"old schema":  {func(d *storage.DatabaseState) { d.SchemaVersion = 4 }, ProblemMigrationPending},
		"no model":    {func(d *storage.DatabaseState) { d.EmbeddingModel = "" }, ProblemNone},
	}
	for name, c := range cases {
		database := currentDatabase()
		c.change(&database)
		got := checkDatabase(healthyConfig(), database)
		if got.Problem != c.want || got.ConfiguredModel != "nomic.gguf" {
			t.Errorf("%s: expected problem %d, got %+v", name, c.want, got)
		}
	}
}

func TestProblemSeverity(t *testing.T) {
	cases := map[Problem]Severity{
		ProblemNone: SeverityOK, ProblemNoConfigFile: SeverityWarning, ProblemMigrationPending: SeverityWarning,
		ProblemMissing: SeverityFailure, ProblemReindexPending: SeverityFailure,
	}
	for problem, want := range cases {
		if got := problem.Severity(); got != want {
			t.Errorf("problem %d: expected severity %d, got %d", problem, want, got)
		}
	}
}
