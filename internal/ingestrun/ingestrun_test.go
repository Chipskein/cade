package ingestrun

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/testcheck"
)

func noEnvironment(string) string { return "" }

func homeAt(path string) func() (string, error) {
	return func() (string, error) { return path, nil }
}

func TestDefaultDirFollowsXDGStateHome(t *testing.T) {
	getenv := func(string) string { return "/xdg/state" }
	dir, err := DefaultDir(getenv, homeAt("/home/ana"))
	if err != nil || dir != "/xdg/state/cade" {
		t.Fatalf("expected /xdg/state/cade, got %q (err %v)", dir, err)
	}
}

func TestDefaultDirFallsBackToLocalState(t *testing.T) {
	dir, err := DefaultDir(noEnvironment, homeAt("/home/ana"))
	if err != nil || dir != "/home/ana/.local/state/cade" {
		t.Fatalf("expected ~/.local/state/cade, got %q (err %v)", dir, err)
	}
}

func TestDefaultDirReportsMissingHome(t *testing.T) {
	missing := func() (string, error) { return "", errors.New("$HOME is not defined") }
	if _, err := DefaultDir(noEnvironment, missing); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("expected the missing home reported, got %v", err)
	}
}

func TestDirPaths(t *testing.T) {
	dir := Dir("/state/cade")
	paths := []string{dir.StatePath(), dir.LockPath(), dir.LogPath()}
	want := []string{"/state/cade/ingest-state.json", "/state/cade/ingest.lock", "/state/cade/ingest.log"}
	if !slices.Equal(paths, want) {
		t.Fatalf("expected %v, got %v", want, paths)
	}
}

func TestStateFileReadsWhatItWrote(t *testing.T) {
	file := JSONStateFile{Dir: Dir(filepath.Join(t.TempDir(), "cade"))}
	written := State{PID: 42, Args: []string{"file", "/notes"}, Mode: Mode{Background: true, Gentle: true}, Status: StatusRunning,
		Stage: StageIngesting, StartedAt: time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC), Progress: "file /notes: 3 read"}
	testcheck.NoError(t, file.Write(written))
	read, found, err := file.Read()
	if err != nil || !found || read.PID != 42 || !read.StartedAt.Equal(written.StartedAt) || read.Progress != written.Progress || !read.Mode.Background {
		t.Fatalf("expected %+v back, got %+v (found %v, err %v)", written, read, found, err)
	}
}

func TestStateFileIsOwnerOnly(t *testing.T) {
	dir := Dir(filepath.Join(t.TempDir(), "cade"))
	testcheck.NoError(t, JSONStateFile{Dir: dir}.Write(State{PID: 1}))
	info, err := os.Stat(dir.StatePath())
	testcheck.NoError(t, err)
	if info.Mode().Perm() != stateFileMode {
		t.Fatalf("expected mode %o, got %o", stateFileMode, info.Mode().Perm())
	}
}

func TestStateFileWithoutRunIsNotFound(t *testing.T) {
	_, found, err := JSONStateFile{Dir: Dir(t.TempDir())}.Read()
	if err != nil || found {
		t.Fatalf("expected no state and no error, got found %v, err %v", found, err)
	}
}

func TestStateFileRejectsDamagedFile(t *testing.T) {
	dir := Dir(t.TempDir())
	testcheck.NoError(t, os.WriteFile(dir.StatePath(), []byte("{"), 0o600))
	if _, _, err := (JSONStateFile{Dir: dir}).Read(); err == nil || !strings.Contains(err.Error(), "parse ingest state") {
		t.Fatalf("expected a parse error naming the file, got %v", err)
	}
}

func TestFileLockRefusesASecondHolder(t *testing.T) {
	lock := FileLock{Dir: Dir(t.TempDir())}
	release, err := lock.Acquire()
	testcheck.NoError(t, err)
	if _, err := lock.Acquire(); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked while held, got %v", err)
	}
	release()
	again, err := lock.Acquire()
	if err != nil {
		t.Fatalf("expected the lock free after release, got %v", err)
	}
	again()
}

func aliveOnly(pids ...int) func(int) bool {
	return func(pid int) bool { return slices.Contains(pids, pid) }
}

func TestStateIsLiveOnlyWhileItsProcessRuns(t *testing.T) {
	running := State{PID: 7, Status: StatusRunning}
	if !running.IsLive(aliveOnly(7)) || running.IsLive(aliveOnly()) {
		t.Fatal("expected a running state live only while pid 7 is alive")
	}
	if (State{PID: 7, Status: StatusFinished}).IsLive(aliveOnly(7)) {
		t.Fatal("expected a finished run not live even if its pid was reused")
	}
}

func TestPausedStateIsLiveAndResumable(t *testing.T) {
	paused := State{PID: 7, Status: StatusPaused}
	if !paused.IsLive(aliveOnly(7)) || !paused.IsPaused(aliveOnly(7)) || !paused.IsResumable(aliveOnly(7)) {
		t.Fatal("expected a paused run with its process alive to be live, paused and resumable")
	}
	if paused.IsLive(aliveOnly()) || paused.IsPaused(aliveOnly()) || !paused.IsResumable(aliveOnly()) {
		t.Fatal("expected a paused run whose process is gone to be neither live nor paused, but resumable")
	}
}

func TestStateIsResumableWhenItStoppedEarly(t *testing.T) {
	cases := map[Status]bool{StatusInterrupted: true, StatusFailed: true, StatusFinished: false}
	for status, want := range cases {
		if got := (State{PID: 7, Status: status}).IsResumable(aliveOnly()); got != want {
			t.Fatalf("expected resumable=%v for %s, got %v", want, status, got)
		}
	}
	if !(State{PID: 7, Status: StatusRunning}).IsResumable(aliveOnly()) {
		t.Fatal("expected a running state whose process is gone to be resumable")
	}
	if (State{PID: 7, Status: StatusRunning}).IsResumable(aliveOnly(7)) {
		t.Fatal("expected a live run not resumable")
	}
}
