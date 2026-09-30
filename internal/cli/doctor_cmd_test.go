package cli

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/doctor"
	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

// ggufStringType is the GGUF metadata value-type tag for a string.
const ggufStringType = 8

// ggufWithSizeLabel builds a minimal GGUF file whose only metadata entry
// is "general.size_label", for testing the vision-pairing warning.
func ggufWithSizeLabel(size string) string {
	var buf bytes.Buffer
	buf.WriteString("GGUF")
	writeGGUFUint32(&buf, 3)
	writeGGUFUint64(&buf, 0)
	writeGGUFUint64(&buf, 1)
	writeGGUFString(&buf, "general.size_label")
	writeGGUFUint32(&buf, ggufStringType)
	writeGGUFString(&buf, size)
	return buf.String()
}

func writeGGUFUint32(buf *bytes.Buffer, v uint32) { _ = binary.Write(buf, binary.LittleEndian, v) }
func writeGGUFUint64(buf *bytes.Buffer, v uint64) { _ = binary.Write(buf, binary.LittleEndian, v) }

func writeGGUFString(buf *bytes.Buffer, s string) {
	writeGGUFUint64(buf, uint64(len(s)))
	buf.WriteString(s)
}

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
	if code != 1 || !strings.Contains(stdout, "rode `go tool mage models` ou ajuste `generation.model_path`") || !strings.Contains(stderr, "1 problema a corrigir") {
		t.Fatalf("expected the missing model and exit 1, got %d %q %q", code, stdout, stderr)
	}
}

func TestDoctorFailsOnMissingProjectorWithImagesOn(t *testing.T) {
	world := doctorWorld()
	world.cfg.Sources.Images, world.cfg.Vision.ProjectorPath = true, "/home/ana/models/mmproj.gguf"
	code, stdout, _ := world.run("doctor")
	if code != 1 || !strings.Contains(stdout, "projetor de visão") || !strings.Contains(stdout, "ajuste `vision.projector_path`") {
		t.Fatalf("expected the missing projector and how to get it, got %d %q", code, stdout)
	}
}

func TestDoctorWarnsOnVisionPairingMismatch(t *testing.T) {
	world := doctorWorld()
	world.cfg.Sources.Images = true
	world.cfg.Generation.ModelPath = "/home/ana/models/qwen-4b.gguf"
	world.cfg.Vision.ProjectorPath = "/home/ana/models/mmproj-qwen-2b.gguf"
	world.files.
		AddFile(world.cfg.Generation.ModelPath, ggufWithSizeLabel("4B")).
		AddFile(world.cfg.Vision.ProjectorPath, ggufWithSizeLabel("2B"))
	code, stdout, _ := world.run("doctor")
	if code != 0 || !strings.Contains(stdout, "tamanho 2B, mas o modelo de geração \"qwen-4b.gguf\" é 4B") {
		t.Fatalf("expected a pairing-mismatch warning naming both models, got %d %q", code, stdout)
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

func TestDoctorShowsTheLastFinishedIngestion(t *testing.T) {
	world := doctorWorld()
	world.runState.State = ingestrun.State{PID: 7, Args: []string{"all"}, Status: ingestrun.StatusFinished,
		StartedAt: cliNow.Add(-50 * time.Hour), FinishedAt: cliNow.Add(-49 * time.Hour)}
	world.runState.Found = true
	code, stdout, _ := world.run("doctor")
	want := "ok     última ingestão        2026-09-24 11:00:00 (há 2 dias), concluída: cade ingest all\n"
	if code != 0 || !strings.Contains(stdout, want) || !strings.Contains(stdout, "Tudo pronto (0 avisos)") {
		t.Fatalf("expected %q and no warning, got %d:\n%s", want, code, stdout)
	}
}

func TestDoctorWarnsAboutAnUnfinishedIngestion(t *testing.T) {
	world := doctorWorld()
	world.runState.State = ingestrun.State{PID: 7, Args: []string{"file"}, Status: ingestrun.StatusInterrupted,
		StartedAt: cliNow.Add(-2 * time.Hour), FinishedAt: cliNow.Add(-time.Hour)}
	world.runState.Found = true
	code, stdout, _ := world.run("doctor")
	if code != 0 || !strings.Contains(stdout, "aviso  última ingestão        2026-09-26 11:00:00 (há 1:00:00), interrompida: cade ingest file") ||
		!strings.Contains(stdout, "cade ingest resume") || !strings.Contains(stdout, "Tudo pronto (1 aviso)") {
		t.Fatalf("expected a warning with a resume hint, got %d:\n%s", code, stdout)
	}
}

func TestDoctorSaysWhenNoIngestionWasRecorded(t *testing.T) {
	world := doctorWorld()
	world.language = English
	_, stdout, _ := world.run("doctor")
	if !strings.Contains(stdout, "—      last ingestion         none recorded\n") || !strings.Contains(stdout, "All set (0 warnings)") {
		t.Fatalf("expected an informative line without a warning, got:\n%s", stdout)
	}
}

func TestDoctorShowsAPausedIngestionWithoutWarning(t *testing.T) {
	world := doctorWorld()
	state := runningState(77)
	state.Status = ingestrun.StatusPaused
	world.runState.State, world.runState.Found = state, true
	world.processes.AlivePIDs = []int{77}
	_, stdout, _ := world.run("doctor")
	if !strings.Contains(stdout, "ok     última ingestão        2026-09-26 11:59:58 (há 0:02), pausada: cade ingest file /notes") ||
		!strings.Contains(stdout, "Tudo pronto (0 avisos)") {
		t.Fatalf("expected a paused run shown without a warning, got:\n%s", stdout)
	}
}

func TestDoctorWarnsAboutAVanishedIngestion(t *testing.T) {
	world := doctorWorld()
	world.runState.State, world.runState.Found = runningState(77), true
	_, stdout, _ := world.run("doctor")
	if !strings.Contains(stdout, "aviso  última ingestão") || !strings.Contains(stdout, "parou sem registrar o fim: cade ingest file /notes") {
		t.Fatalf("expected a warning for a run whose process is gone, got:\n%s", stdout)
	}
}
