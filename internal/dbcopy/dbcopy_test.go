package dbcopy

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/chipskein/cade/internal/rootfs"
)

var copiedAt = time.Date(2026, 10, 11, 10, 15, 0, 0, time.UTC)

func TestNamesSayWhatTheCopyWasFor(t *testing.T) {
	if got := BeforeMigration("/data/cade.db", 18, copiedAt); got != "/data/cade.db.before-v18-20261011-101500" {
		t.Errorf("BeforeMigration = %q", got)
	}
	if got := BeforeRekey("/data/cade.db", copiedAt); got != "/data/cade.db.before-rekey-20261011-101500" {
		t.Errorf("BeforeRekey = %q", got)
	}
}

func TestFindListsOnlyTheCopiesOldestFirst(t *testing.T) {
	file := func(at time.Time, size int) *fstest.MapFile {
		return &fstest.MapFile{Data: make([]byte, size), ModTime: at}
	}
	fsys := fstest.MapFS{
		rootfs.Name(BeforeRekey("/data/cade.db", copiedAt)):           file(copiedAt, 3),
		rootfs.Name(BeforeMigration("/data/cade.db", 15, copiedAt)):   file(copiedAt.AddDate(0, 0, -5), 7),
		rootfs.Name("/data/cade.db"):                                  file(copiedAt, 9),
		rootfs.Name("/data/cade.db-wal"):                              file(copiedAt, 1),
		rootfs.Name("/data/other.db.before-v15-20261001-000000"):      file(copiedAt, 1),
		rootfs.Name("/data/cade.db.before-v16-20261001-000000/inner"): file(copiedAt, 1),
	}
	copies, err := Find(fsys, "/data/cade.db")
	if err != nil || len(copies) != 2 || copies[0].Size != 7 || copies[1].Path != "/data/cade.db.before-rekey-20261011-101500" {
		t.Fatalf("expected the migration copy then the rekey copy, got %+v (err %v)", copies, err)
	}
}

func TestFindInAMissingDirectoryFails(t *testing.T) {
	if _, err := Find(fstest.MapFS{}, "/absent/cade.db"); err == nil {
		t.Fatal("expected an error")
	}
}
