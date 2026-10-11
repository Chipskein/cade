package cli

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chipskein/cade/internal/dbcopy"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/rootfs"
)

const copiesDatabase = "/data/cade.db"

// copiesWorld has a database with one commit and, when withCopies, a copy
// from before a migration and one from before a rekey.
func copiesWorld(withCopies bool) (*fakeWorld, []string) {
	world := newFakeWorld()
	world.cfg.DatabasePath = copiesDatabase
	world.store.Events = []event.Event{{UID: "a", Source: event.SourceGit, Timestamp: cliNow, Content: "café"}}
	world.files.MapFS[rootfs.Name(copiesDatabase)] = &fstest.MapFile{Data: []byte("db"), ModTime: cliNow}
	if !withCopies {
		return world, nil
	}
	copies := []string{dbcopy.BeforeMigration(copiesDatabase, 18, cliNow.AddDate(0, 0, -3)), dbcopy.BeforeRekey(copiesDatabase, cliNow)}
	for i, path := range copies {
		world.files.MapFS[rootfs.Name(path)] = &fstest.MapFile{Data: make([]byte, 3<<20), ModTime: cliNow.AddDate(0, 0, i-3)}
	}
	return world, copies
}

func TestForgetListsTheCopiesThatStillHoldWhatWasDeleted(t *testing.T) {
	world, copies := copiesWorld(true)
	code, _, stderr := world.run("forget", "--uid", "a")
	if code != 0 || !strings.Contains(stderr, "2 cópias do banco ainda guardam o que foi apagado") || !strings.Contains(stderr, copies[0]+"  (") ||
		!strings.Contains(stderr, "3 MB") || !strings.Contains(stderr, "cade forget --backups") {
		t.Fatalf("expected both copies listed with size and how to delete them, got %d %q", code, stderr)
	}
}

func TestForgetWithoutCopiesSaysNothingAboutThem(t *testing.T) {
	world, _ := copiesWorld(false)
	if code, _, stderr := world.run("forget", "--uid", "a"); code != 0 || strings.Contains(stderr, "cópia") {
		t.Fatalf("expected no word about copies, got %d %q", code, stderr)
	}
}

// Acceptance (#87): deleting the copies asks first and never touches the
// database.
func TestForgetBackupsDeletesTheCopiesAndNeverTheDatabase(t *testing.T) {
	world, copies := copiesWorld(true)
	code, stdout, stderr := world.run("forget", "--backups", "--yes")
	if code != 0 || !slices.Equal(world.removedFiles, copies) || !strings.Contains(stdout, "2 cópias apagadas.") {
		t.Fatalf("expected both copies deleted, got %d %v %q %q", code, world.removedFiles, stdout, stderr)
	}
	if _, kept := world.files.MapFS[rootfs.Name(copiesDatabase)]; !kept || len(world.store.Events) != 1 {
		t.Fatal("expected the database and its events untouched")
	}
}

func TestForgetBackupsOutsideATerminalNeedsYes(t *testing.T) {
	world, _ := copiesWorld(true)
	if code, _, _ := world.run("forget", "--backups"); code == 0 || len(world.removedFiles) != 0 {
		t.Fatalf("expected a refusal and nothing deleted, got %d %v", code, world.removedFiles)
	}
}

func TestForgetBackupsWithoutCopies(t *testing.T) {
	world, _ := copiesWorld(false)
	if code, stdout, _ := world.run("forget", "--backups", "--yes"); code != 0 || !strings.Contains(stdout, "Nenhuma cópia do banco.") {
		t.Fatalf("expected the absence said, got %d %q", code, stdout)
	}
}

func TestMegabytesOfRoundsUp(t *testing.T) {
	if megabytesOf(1) != 1 || megabytesOf(1<<20) != 1 || megabytesOf(1<<20+1) != 2 || megabytesOf(0) != 0 {
		t.Fatal("expected sizes rounded up to whole megabytes")
	}
}

func TestDoctorShowsTheCopiesWithoutWarning(t *testing.T) {
	world := doctorWorld()
	world.files.AddFile(dbcopy.BeforeMigration("/home/ana/cade.db", 18, cliNow), strings.Repeat("x", 2<<20))
	world.files.MapFS[rootfs.Name(dbcopy.BeforeMigration("/home/ana/cade.db", 18, cliNow))].ModTime = cliNow.AddDate(0, 0, -30)
	code, stdout, _ := world.run("doctor")
	if code != 0 || !strings.Contains(stdout, "cópias do banco        1, a mais antiga de") || !strings.Contains(stdout, "(há 30 dias), 2 MB") ||
		!strings.Contains(stdout, "cade forget --backups") || !strings.Contains(stdout, "Tudo pronto (0 avisos)") {
		t.Fatalf("expected the copy's line and a clean verdict, got %d %q", code, stdout)
	}
}
