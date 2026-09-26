package gitsource

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

// FakeGitRunner returns canned git output and records the arguments used.
type FakeGitRunner struct {
	Output   string
	FailWith error
	LastArgs []string
}

func (f *FakeGitRunner) Run(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
	f.LastArgs = args
	return []byte(f.Output), f.FailWith
}

const twoCommitsLog = "\x1eaaa111\x1f2026-09-25T14:03:00-03:00\x1fAna\x1fana@x.io\x1fFix login\n\nlong body\n\x1f\n\nauth/login.go\nauth/login_test.go\n" +
	"\x1ebbb222\x1f2026-09-24T09:00:00-03:00\x1fAna\x1fana@x.io\x1fMerge branch\x1f\n"

func collect(t *testing.T, runner CommandRunner) ([]event.Event, error) {
	t.Helper()
	var events []event.Event
	err := NewCollector(runner, "/repo", []string{"ana@x.io"}).CollectEvents(context.Background(), func(ev event.Event) error {
		events = append(events, ev)
		return nil
	})
	return events, err
}

func TestCollectEventsParsesCommits(t *testing.T) {
	events, err := collect(t, &FakeGitRunner{Output: twoCommitsLog})
	if err != nil || len(events) != 2 {
		t.Fatalf("expected 2 events, got %d (err %v)", len(events), err)
	}
	first := events[0]
	if first.Metadata["hash"] != "aaa111" || first.Metadata["files"] != "auth/login.go\nauth/login_test.go" || first.Source != event.SourceGit {
		t.Fatalf("unexpected metadata %+v", first.Metadata)
	}
	if first.Timestamp.UTC().Hour() != 17 || !strings.HasPrefix(first.Content, "Fix login\n\nlong body") {
		t.Fatalf("unexpected timestamp %s or content %q", first.Timestamp, first.Content)
	}
}

func TestCollectEventsUsesHashAsStableID(t *testing.T) {
	events, _ := collect(t, &FakeGitRunner{Output: twoCommitsLog})
	if events[0].UID != event.StableID(event.SourceGit, "aaa111") {
		t.Fatalf("expected UID derived from hash, got %q", events[0].UID)
	}
}

func TestCollectEventsPassesAuthorFilter(t *testing.T) {
	runner := &FakeGitRunner{}
	collect(t, runner)
	if !slices.Contains(runner.LastArgs, "--author=ana@x.io") {
		t.Fatalf("expected author filter in %v", runner.LastArgs)
	}
}

func TestCollectEventsWrapsRunnerError(t *testing.T) {
	_, err := collect(t, &FakeGitRunner{FailWith: errors.New("not a git repository")})
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
}

func TestParseCommitRejectsTruncatedRecord(t *testing.T) {
	if _, err := parseCommit("abc\x1fdate", "/repo"); err == nil {
		t.Fatal("expected an error for a record with missing fields")
	}
}

func TestParseCommitRejectsBadDate(t *testing.T) {
	if _, err := parseCommit("h\x1fyesterday\x1fa\x1fe\x1fmsg\x1f", "/repo"); err == nil {
		t.Fatal("expected an error for a non-RFC3339 date")
	}
}

func TestCommitContentWithoutFiles(t *testing.T) {
	if got := commitContent("msg", nil); got != "msg" {
		t.Fatalf("expected bare message, got %q", got)
	}
}

func TestChangedFilesSkipsBlankLines(t *testing.T) {
	if got := changedFiles("\n\na.go\n\nb.go\n"); len(got) != 2 || got[1] != "b.go" {
		t.Fatalf("expected [a.go b.go], got %v", got)
	}
}

// Exercises the real git binary against a throwaway repository, verifying
// that the log format parses what git actually prints.
func TestCollectEventsAgainstRealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repository := initRepositoryWithCommit(t)
	var events []event.Event
	err := NewCollector(ExecRunner{}, repository, nil).CollectEvents(context.Background(), func(ev event.Event) error {
		events = append(events, ev)
		return nil
	})
	if err != nil || len(events) != 1 || events[0].Metadata["files"] != "notes.txt" {
		t.Fatalf("expected one commit touching notes.txt, got %+v (err %v)", events, err)
	}
}

func initRepositoryWithCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o600)
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "."},
		{"-c", "user.name=T", "-c", "user.email=t@t", "commit", "-q", "-m", "first commit"},
	} {
		if _, err := (ExecRunner{}).Run(context.Background(), dir, "git", args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return dir
}

func TestExecRunnerIncludesStderrInError(t *testing.T) {
	_, err := ExecRunner{}.Run(context.Background(), t.TempDir(), "git", "log")
	if err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("expected an error mentioning git, got %v", err)
	}
}
