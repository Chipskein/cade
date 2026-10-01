package doctor

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

// ggufStringType is the GGUF metadata value-type tag for a string.
const ggufStringType = 8

// ggufWithSizeLabel builds a minimal GGUF file whose only metadata entry
// is "general.size_label", for testing the vision-pairing check.
func ggufWithSizeLabel(size string) string {
	var buf bytes.Buffer
	buf.WriteString("GGUF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))
	writeGGUFString(&buf, "general.size_label")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(ggufStringType))
	writeGGUFString(&buf, size)
	return buf.String()
}

func writeGGUFString(buf *bytes.Buffer, s string) {
	_ = binary.Write(buf, binary.LittleEndian, uint64(len(s)))
	buf.WriteString(s)
}

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

func TestVisionProjectorCheckedOnlyWithImagesOn(t *testing.T) {
	cfg := healthyConfig()
	cfg.Vision.ProjectorPath = "/models/absent-mmproj.gguf"
	for _, finding := range Diagnose(healthyInstall(), configPath, cfg, currentDatabase()) {
		if finding.Subject == SubjectVisionProjector {
			t.Fatalf("expected no projector check with images off, got %+v", finding)
		}
	}
	cfg.Sources.Images = true
	got := findingFor(t, Diagnose(healthyInstall(), configPath, cfg, currentDatabase()), SubjectVisionProjector, "/models/absent-mmproj.gguf")
	if got.Problem != ProblemMissing || got.Setting != "vision.projector_path" {
		t.Fatalf("expected the missing projector reported with images on, got %+v", got)
	}
}

func TestVisionProjectorPairingMismatchIsWarned(t *testing.T) {
	cfg := healthyConfig()
	cfg.Sources.Images = true
	cfg.Generation.ModelPath = "/models/qwen-4b.gguf"
	cfg.Vision.ProjectorPath = "/models/mmproj-qwen-2b.gguf"
	fsys := healthyInstall().
		AddFile(cfg.Generation.ModelPath, ggufWithSizeLabel("4B")).
		AddFile(cfg.Vision.ProjectorPath, ggufWithSizeLabel("2B"))
	findings := Diagnose(fsys, configPath, cfg, currentDatabase())
	got := mismatchFinding(t, findings)
	if got.PairedModelSize != "4B" || got.ProjectorSize != "2B" || got.PairedModelPath != cfg.Generation.ModelPath {
		t.Fatalf("expected a pairing mismatch naming both sizes, got %+v", got)
	}
}

// mismatchFinding is the one SubjectVisionProjector finding reporting
// ProblemVisionModelMismatch; both it and the plain projector-file check
// share a subject and path, so findingFor can't tell them apart.
func mismatchFinding(t *testing.T, findings []Finding) Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.Subject == SubjectVisionProjector && finding.Problem == ProblemVisionModelMismatch {
			return finding
		}
	}
	t.Fatalf("no vision pairing mismatch finding in %+v", findings)
	return Finding{}
}

func TestVisionProjectorPairingMatchHasNoProblem(t *testing.T) {
	cfg := healthyConfig()
	cfg.Sources.Images = true
	cfg.Generation.ModelPath = "/models/qwen-2b.gguf"
	cfg.Vision.ProjectorPath = "/models/mmproj-qwen-2b.gguf"
	fsys := healthyInstall().
		AddFile(cfg.Generation.ModelPath, ggufWithSizeLabel("2B")).
		AddFile(cfg.Vision.ProjectorPath, ggufWithSizeLabel("2B"))
	for _, finding := range Diagnose(fsys, configPath, cfg, currentDatabase()) {
		if finding.Subject == SubjectVisionProjector && finding.Problem != ProblemNone {
			t.Fatalf("expected matching sizes to report no problem, got %+v", finding)
		}
	}
}

func TestVisionProjectorPairingSkippedWithoutSizeLabels(t *testing.T) {
	cfg := healthyConfig()
	cfg.Sources.Images = true
	cfg.Vision.ProjectorPath = "/models/mmproj.gguf"
	fsys := healthyInstall().AddFile(cfg.Vision.ProjectorPath, "GGUF\x03rest")
	got := findingFor(t, Diagnose(fsys, configPath, cfg, currentDatabase()), SubjectVisionProjector, cfg.Vision.ProjectorPath)
	if got.Problem != ProblemNone {
		t.Fatalf("expected no problem when neither file carries a size_label, got %+v", got)
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
		"few empty":   {func(d *storage.DatabaseState) { d.VectorSlots = storage.VectorSlots{Slots: 1024, Vectors: 1000} }, ProblemNone},
		"many empty":  {func(d *storage.DatabaseState) { d.VectorSlots = storage.VectorSlots{Slots: 2048, Vectors: 1000} }, ProblemEmptyVectorSlots},
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

func TestThresholdCalibrationFindings(t *testing.T) {
	cfg := config.Defaults()
	cfg.Embedding.ModelPath = "/models/nomic.gguf"
	defaults := cfg.Retrieval
	cases := map[string]struct {
		calibration storage.ThresholdCalibration
		want        Problem
	}{
		"set for this model":       {storage.ThresholdCalibration{Model: "nomic.gguf", MaxDistance: defaults.MaxDistance, MaxBestDistance: defaults.MaxBestDistance}, ProblemNone},
		"left from another model":  {storage.ThresholdCalibration{Model: "bge.gguf", MaxDistance: defaults.MaxDistance, MaxBestDistance: defaults.MaxBestDistance}, ProblemThresholdModelMismatch},
		"retuned after the change": {storage.ThresholdCalibration{Model: "bge.gguf", MaxDistance: 0.5, MaxBestDistance: defaults.MaxBestDistance}, ProblemNone},
		"no record, same vectors":  {storage.ThresholdCalibration{}, ProblemNone},
	}
	for name, c := range cases {
		database := currentDatabase()
		database.ThresholdCalibration = c.calibration
		if got := checkThresholdCalibration(cfg, database); got.Problem != c.want {
			t.Errorf("%s: expected problem %v, got %+v", name, c.want, got)
		}
	}
}

// A database from before the record adopts its vectors' model.
func TestThresholdCalibrationAdoptsIndexedModel(t *testing.T) {
	cfg := config.Defaults()
	cfg.Embedding.ModelPath = "/models/bge.gguf"
	got := checkThresholdCalibration(cfg, currentDatabase())
	if got.Problem != ProblemThresholdModelMismatch || got.Database.ThresholdCalibration.Model != "nomic.gguf" {
		t.Fatalf("expected gates attributed to nomic.gguf, got %+v", got)
	}
}

func TestProblemSeverity(t *testing.T) {
	cases := map[Problem]Severity{
		ProblemNone: SeverityOK, ProblemNoConfigFile: SeverityWarning, ProblemMigrationPending: SeverityWarning, ProblemEmptyVectorSlots: SeverityWarning,
		ProblemMissing: SeverityFailure, ProblemReindexPending: SeverityFailure,
	}
	for problem, want := range cases {
		if got := problem.Severity(); got != want {
			t.Errorf("problem %d: expected severity %d, got %d", problem, want, got)
		}
	}
}
