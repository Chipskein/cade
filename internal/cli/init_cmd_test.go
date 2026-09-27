package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testfakes"
)

// fakeMachine has two browser profiles, a Teams cache and three
// repositories under ~/src, one of them in node_modules.
func fakeMachine() testfakes.FakeFileSystem {
	return testfakes.NewFakeFileSystem().
		AddFile("/home/ana/.config/google-chrome/Default/History", "").
		AddDir("/home/ana/.config/google-chrome/Default/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb").
		AddFile("/home/ana/.mozilla/firefox/x1.default/places.sqlite", "").
		AddDir("/home/ana/src/api/.git").
		AddDir("/home/ana/src/web/.git").
		AddDir("/home/ana/src/web/node_modules/lib/.git")
}

func initWorld(stdin string) *fakeWorld {
	world := newFakeWorld()
	world.files, world.stdin = fakeMachine(), stdin
	return world
}

func TestInitWithoutAnswersTakesDefaults(t *testing.T) {
	world := initWorld("")
	code, stdout, stderr := world.run("init")
	sources := world.writtenCfg.Sources
	want := []string{"~/.config/google-chrome/Default/History", "~/.mozilla/firefox/x1.default/places.sqlite"}
	if code != 0 || !slices.Equal(sources.BrowserHistories, want) {
		t.Fatalf("expected both histories by default, got %d %v %q", code, sources.BrowserHistories, stderr)
	}
	if len(sources.TeamsIndexedDBDirs) != 0 || len(sources.GitRepositories) != 0 || len(sources.Directories) != 0 {
		t.Fatalf("expected no Teams, repositories or folders by default, got %+v", sources)
	}
	if !strings.Contains(stdout, "Configuração criada em /cfg/config.json") || !strings.Contains(stdout, "cade doctor") {
		t.Fatalf("expected the summary and next steps, got %q", stdout)
	}
}

func TestInitRecordsEveryAnswer(t *testing.T) {
	world := initWorld("2\nt\n~/src\n1\n~/notas\n/srv/docs\n\n")
	if code, _, stderr := world.run("init"); code != 0 {
		t.Fatalf("expected success, got %d %q", code, stderr)
	}
	sources := world.writtenCfg.Sources
	if !slices.Equal(sources.BrowserHistories, []string{"~/.mozilla/firefox/x1.default/places.sqlite"}) ||
		!slices.Equal(sources.TeamsIndexedDBDirs, []string{"~/.config/google-chrome/Default/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb"}) {
		t.Fatalf("expected the second history and the Teams cache, got %+v", sources)
	}
	if !slices.Equal(sources.GitRepositories, []string{"~/src/api"}) || !slices.Equal(sources.Directories, []string{"~/notas", "/srv/docs"}) {
		t.Fatalf("expected ~/src/api and two folders, got %+v", sources)
	}
}

func TestInitWarnsBeforeOfferingTeams(t *testing.T) {
	_, stdout, _ := initWorld("").run("init")
	warning, offer := strings.Index(stdout, "política de dados"), strings.Index(stdout, "Caches do Teams")
	if warning < 0 || offer < warning {
		t.Fatalf("expected the data policy note before the Teams list, got %q", stdout)
	}
}

func TestInitAsksAgainAfterAnUnclearAnswer(t *testing.T) {
	world := initWorld("9\nn\n")
	_, stdout, _ := world.run("init")
	if !strings.Contains(stdout, "Resposta não entendida.") || len(world.writtenCfg.Sources.BrowserHistories) != 0 {
		t.Fatalf("expected a retry and no histories, got %q %+v", stdout, world.writtenCfg.Sources)
	}
}

func TestInitReportsNothingFound(t *testing.T) {
	world := newFakeWorld()
	world.stdin = "/empty\n"
	_, stdout, _ := world.run("init")
	if strings.Count(stdout, "nenhum encontrado") != 3 || world.writtenCfg.Sources.BrowserHistories == nil {
		t.Fatalf("expected histories, Teams and repositories reported as not found, got %q", stdout)
	}
}

func TestInitRefusesExistingConfig(t *testing.T) {
	world := initWorld("")
	world.files.AddFile("/cfg/config.json", "{}")
	code, _, stderr := world.run("init")
	if code != 1 || world.writtenConfig != "" || !strings.Contains(stderr, "já existe") {
		t.Fatalf("expected a refusal without writing, got %d %q", code, stderr)
	}
}

func TestInitRejectsArguments(t *testing.T) {
	if code, _, _ := initWorld("").run("init", "extra"); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestInitInEnglish(t *testing.T) {
	world := initWorld("")
	world.language = English
	_, stdout, _ := world.run("init")
	if !strings.Contains(stdout, "Browser histories:") || !strings.Contains(stdout, "Config written to") || strings.Contains(stdout, "Configuração") {
		t.Fatalf("expected English prompts, got %q", stdout)
	}
}
