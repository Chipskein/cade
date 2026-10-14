package devtasks

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const driverDownloadCommand = "go mod download -json github.com/mattn/go-sqlite3@v1.14.52"

// driverWorld is a testWorld without the driver built, whose module cache
// holds a fake go-sqlite3 and whose fetcher serves a fake amalgamation.
type driverWorld struct {
	*testWorld
	archive pinnedArchive
	fetcher FakeArchiveFetcher
}

func newDriverWorld(t *testing.T) *driverWorld {
	t.Helper()
	world := &driverWorld{testWorld: newTestWorld(t)}
	if err := os.RemoveAll(filepath.Join(world.root, sqlite3mcDir)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sqlite3.go", "sqlite3_test.go", "sqlite3-binding.c", "sqlite3ext.h", "go.mod", "LICENSE", "README.md", "_example/main.go"} {
		world.writeFile(t, "modcache/"+name, "upstream "+name)
	}
	download, err := json.Marshal(moduleDownload{Dir: filepath.Join(world.root, "modcache")})
	if err != nil {
		t.Fatal(err)
	}
	world.runner.Outputs[driverDownloadCommand] = string(download)
	world.fetcher.Body = amalgamationZip(t, map[string]string{"sqlite3mc_amalgamation.c": "mc code", "sqlite3mc_amalgamation.h": "mc header"})
	sum := sha256.Sum256(world.fetcher.Body)
	world.archive = pinnedArchive{url: "https://example.test/sqlite3mc.zip", sha256: hex.EncodeToString(sum[:])}
	return world
}

func (w *driverWorld) tasks() *Tasks {
	return NewTasks(Dependencies{Root: w.root, Runner: w.runner, Fetcher: w.fetcher, Env: w.env, Progress: w.progress})
}

func (w *driverWorld) read(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(w.root, sqlite3mcDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func amalgamationZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	zipped := zip.NewWriter(&archive)
	for name, content := range files {
		writer, err := zipped.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func TestEnsureSQLite3MCSkipsACurrentDriver(t *testing.T) {
	world := newTestWorld(t)
	if err := world.tasks().EnsureSQLite3MC(); err != nil || len(world.runner.Commands) != 0 || len(world.fetcher.URLs) != 0 {
		t.Errorf("err %v, commands %q, downloads %q; want nothing run", err, world.runner.Lines(), world.fetcher.URLs)
	}
}

func TestBuildSQLite3MCSwapsTheAmalgamationIntoTheDriver(t *testing.T) {
	world := newDriverWorld(t)
	if err := world.tasks().buildSQLite3MC(world.archive); err != nil {
		t.Fatal(err)
	}
	assertLines(t, world.runner.Lines(), driverDownloadCommand)
	if got := world.read(t, "sqlite3-binding.c"); got != "#ifndef USE_LIBSQLITE3\nmc code\n#endif\n" {
		t.Errorf("sqlite3-binding.c = %q, want the sqlite3mc code inside the libsqlite3 guard", got)
	}
	if got := world.read(t, "sqlite3-binding.h"); !strings.Contains(got, "mc header") {
		t.Errorf("sqlite3-binding.h = %q, want the sqlite3mc header", got)
	}
	for _, name := range []string{"sqlite3.go", "sqlite3ext.h", "go.mod", "LICENSE"} {
		if got := world.read(t, name); got != "upstream "+name {
			t.Errorf("%s = %q, want the upstream copy", name, got)
		}
	}
	for _, name := range []string{"sqlite3_test.go", "README.md", "_example"} {
		if _, err := os.Stat(filepath.Join(world.root, sqlite3mcDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s copied (stat err %v), want only the build files", name, err)
		}
	}
	if got := world.read(t, ".cade-stamp"); got != sqlite3mcStamp {
		t.Errorf("stamp = %q, want %q", got, sqlite3mcStamp)
	}
}

func TestBuildSQLite3MCReplacesAStaleDriver(t *testing.T) {
	world := newDriverWorld(t)
	world.writeFile(t, sqlite3mcStampFile, "v1.14.51+sqlite3mc-2.5.0")
	world.writeFile(t, sqlite3mcDir+"/old.go", "")
	if err := world.tasks().buildSQLite3MC(world.archive); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(world.root, sqlite3mcDir, "old.go")); !os.IsNotExist(err) {
		t.Errorf("old.go kept (stat err %v), want the directory rebuilt", err)
	}
}

func TestBuildSQLite3MCRejectsAnArchiveWithAnotherChecksum(t *testing.T) {
	world := newDriverWorld(t)
	world.archive.sha256 = strings.Repeat("0", 64)
	err := world.tasks().buildSQLite3MC(world.archive)
	if err == nil || !strings.Contains(err.Error(), "want "+world.archive.sha256) {
		t.Fatalf("err = %v, want a checksum mismatch naming the pinned SHA-256", err)
	}
	if _, err := os.Stat(filepath.Join(world.root, sqlite3mcDir)); !os.IsNotExist(err) {
		t.Errorf("driver directory written (stat err %v), want nothing", err)
	}
}

func TestBuildSQLite3MCNamesAMissingAmalgamationFile(t *testing.T) {
	world := newDriverWorld(t)
	world.fetcher.Body = amalgamationZip(t, map[string]string{"sqlite3mc_amalgamation.c": "mc code"})
	sum := sha256.Sum256(world.fetcher.Body)
	world.archive.sha256 = hex.EncodeToString(sum[:])
	err := world.tasks().buildSQLite3MC(world.archive)
	if err == nil || !strings.Contains(err.Error(), "sqlite3mc_amalgamation.h") {
		t.Errorf("err = %v, want it to name sqlite3mc_amalgamation.h", err)
	}
}

func TestBuildSQLite3MCRejectsAnUnreadableModuleDownload(t *testing.T) {
	world := newDriverWorld(t)
	world.runner.Outputs[driverDownloadCommand] = `{"Error": "no network"}`
	err := world.tasks().buildSQLite3MC(world.archive)
	if err == nil || !strings.Contains(err.Error(), "no network") {
		t.Errorf("err = %v, want it to carry the go command's error", err)
	}
}

func TestDriverVersionMatchesGoMod(t *testing.T) {
	goMod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goMod), goSQLite3Module+" "+GoSQLite3Version) {
		t.Errorf("go.mod does not require %s %s, the driver the sqlite3mc build copies", goSQLite3Module, GoSQLite3Version)
	}
	if !strings.Contains(string(goMod), "replace "+goSQLite3Module+" => ./"+sqlite3mcDir) {
		t.Errorf("go.mod does not replace %s with ./%s", goSQLite3Module, sqlite3mcDir)
	}
}
