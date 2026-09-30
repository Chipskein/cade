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
	"github.com/chipskein/cade/internal/testcheck"
)

// FakeGitRunner returns canned git output and records the arguments used.
// FakeGitRunner returns canned git log output, answers `git config KEY`
// from Config (an error when absent), and records the log arguments.
type FakeGitRunner struct {
	Output   string
	Config   map[string]string
	FailWith error
	LastArgs []string
}

func (f *FakeGitRunner) Run(_ context.Context, _ string, _ string, args ...string) ([]byte, error) {
	if len(args) == 2 && args[0] == "config" {
		value, found := f.Config[args[1]]
		if !found {
			return nil, errors.New("exit status 1")
		}
		return []byte(value + "\n"), nil
	}
	f.LastArgs = args
	return []byte(f.Output), f.FailWith
}

const twoCommitsLog = "\x1eaaa111\x1f2026-09-25T14:03:00-03:00\x1fAna\x1fana@x.io\x1fFix login\n\nlong body\n\x1f\n\nauth/login.go\nauth/login_test.go\n" +
	"\x1ebbb222\x1f2026-09-24T09:00:00-03:00\x1fAna\x1fana@x.io\x1fMerge branch\x1f\n"

func collect(t *testing.T, runner CommandRunner) ([]event.Event, error) {
	t.Helper()
	var events []event.Event
	err := NewCollector(runner, "/repo", []string{"ana@x.io"}, nil).CollectEvents(context.Background(), func(ev event.Event) error {
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
	if _, err := collect(t, runner); err != nil {
		t.Fatal(err)
	}
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
	if _, err := parseCommit("abc\x1fdate", "/repo", nil); err == nil {
		t.Fatal("expected an error for a record with missing fields")
	}
}

func TestParseCommitRejectsBadDate(t *testing.T) {
	if _, err := parseCommit("h\x1fyesterday\x1fa\x1fe\x1fmsg\x1f", "/repo", nil); err == nil {
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
	err := NewCollector(ExecRunner{}, repository, nil, nil).CollectEvents(context.Background(), func(ev event.Event) error {
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
	testcheck.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o600))
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

func authorshipByHash(t *testing.T, collector *Collector) map[string]event.Authorship {
	t.Helper()
	marks := map[string]event.Authorship{}
	err := collector.CollectEvents(context.Background(), func(ev event.Event) error {
		marks[ev.Commit().Hash] = ev.Commit().Authorship
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return marks
}

const twoAuthorsLog = "\x1eaaa111\x1f2026-09-25T14:03:00-03:00\x1fAna\x1fana@x.io\x1fFix login\x1f\n" +
	"\x1ebbb222\x1f2026-09-24T09:00:00-03:00\x1fRui Costa\x1frui@x.io\x1fRetry do ERP\x1f\n"

// Regression: in a team repository every colleague's commit counted as
// the user's work.
func TestAutoIdentityMarksOwnCommits(t *testing.T) {
	runner := &FakeGitRunner{Output: twoAuthorsLog, Config: map[string]string{"user.email": "ANA@x.io", "user.name": "Ana"}}
	collector := NewCollector(runner, "/repo", nil, []string{AutoIdentity})
	marks := authorshipByHash(t, collector)
	if marks["aaa111"] != event.AuthorshipMine || marks["bbb222"] != event.AuthorshipOther {
		t.Fatalf("expected Ana's commit mine and Rui's other, got %v", marks)
	}
	if repository, identities := collector.CommitAuthorship(); repository != "/repo" || len(identities) != 2 || identities[0] != "ana@x.io" {
		t.Fatalf("expected the repository and lowercased identities, got %q %v", repository, identities)
	}
}

func TestExplicitIdentitiesMatchNameOrEmail(t *testing.T) {
	marks := authorshipByHash(t, NewCollector(&FakeGitRunner{Output: twoAuthorsLog}, "/repo", nil, []string{"Rui Costa"}))
	if marks["bbb222"] != event.AuthorshipMine || marks["aaa111"] != event.AuthorshipOther {
		t.Fatalf("expected the named author's commit mine, got %v", marks)
	}
}

// A repository without a configured user must not mark every commit as
// someone else's.
func TestUnresolvedIdentityLeavesAuthorshipUnknown(t *testing.T) {
	marks := authorshipByHash(t, NewCollector(&FakeGitRunner{Output: twoAuthorsLog}, "/repo", nil, []string{AutoIdentity}))
	if marks["aaa111"] != event.AuthorshipUnknown || marks["bbb222"] != event.AuthorshipUnknown {
		t.Fatalf("expected unknown authorship, got %v", marks)
	}
}

// Acceptance (phase 4, see CHANGELOG.md) against the real git binary: a
// repository with two authors marks only the configured user's commit.
func TestRealRepositoryWithTwoAuthors(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repository := initRepositoryWithCommit(t)
	for _, args := range [][]string{
		{"config", "user.email", "t@t"}, {"config", "user.name", "T"},
		{"-c", "user.name=Rui", "-c", "user.email=rui@x.io", "commit", "-q", "--allow-empty", "-m", "commit do Rui"},
	} {
		if output, err := exec.Command("git", append([]string{"-C", repository}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	var mine, others int
	err := NewCollector(ExecRunner{}, repository, nil, []string{AutoIdentity}).CollectEvents(context.Background(), func(ev event.Event) error {
		mine, others = mine+boolInt(ev.Commit().Authorship == event.AuthorshipMine), others+boolInt(ev.IsOthersCommit())
		return nil
	})
	testcheck.NoError(t, err)
	if mine != 1 || others != 1 {
		t.Fatalf("expected one commit of each, got %d mine and %d others", mine, others)
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestEstimateEventsCountsTheSameCommits(t *testing.T) {
	runner := &FakeGitRunner{Output: "2\n"}
	count, err := NewCollector(runner, "/repo", []string{"ana@x.io"}, nil).EstimateEvents(context.Background())
	want := []string{"rev-list", "--count", "--branches", "--tags", "--remotes", "HEAD", "--author=ana@x.io"}
	if err != nil || count != 2 || !slices.Equal(runner.LastArgs, want) {
		t.Fatalf("expected 2 commits counted with %v, got %d with %v (err %v)", want, count, runner.LastArgs, err)
	}
}

func TestEstimateEventsRejectsOutputThatIsNotACount(t *testing.T) {
	_, err := NewCollector(&FakeGitRunner{Output: "fatal\n"}, "/repo", nil, nil).EstimateEvents(context.Background())
	if err == nil || !strings.Contains(err.Error(), `"fatal\n"`) {
		t.Fatalf("expected the unexpected output quoted, got %v", err)
	}
}
