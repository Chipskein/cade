package imagepreview

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// FakeChafaRunner returns canned output and records the arguments used.
type FakeChafaRunner struct {
	Stdout []byte
	Err    error
	Calls  [][]string
}

func (f *FakeChafaRunner) Output(_ context.Context, program string, args ...string) ([]byte, error) {
	f.Calls = append(f.Calls, append([]string{program}, args...))
	return f.Stdout, f.Err
}

func foundOnPath(program string) (string, error) { return "/usr/bin/" + program, nil }

func missingFromPath(program string) (string, error) {
	return "", errors.New(program + ": executable file not found in $PATH")
}

func newTestPreviewer(runner *FakeChafaRunner, lookPath func(string) (string, error)) Previewer {
	return Previewer{Runner: runner, LookPath: lookPath, Size: PreviewSize{Columns: 30, Rows: 10}}
}

func TestRenderRunsChafaOnImage(t *testing.T) {
	runner := &FakeChafaRunner{Stdout: []byte("▀▀▀\n")}
	got := newTestPreviewer(runner, foundOnPath).Render(context.Background(), "/notes/Print.PNG")
	want := []string{ChafaProgram, "--size=30x10", "--", "/notes/Print.PNG"}
	if got != "▀▀▀" || len(runner.Calls) != 1 || !slices.Equal(runner.Calls[0], want) {
		t.Fatalf("expected trimmed chafa output from %v, got %q from %v", want, got, runner.Calls)
	}
}

func TestRenderSkipsNonImage(t *testing.T) {
	runner := &FakeChafaRunner{Stdout: []byte("art")}
	if got := newTestPreviewer(runner, foundOnPath).Render(context.Background(), "/notes/a.md"); got != "" || len(runner.Calls) != 0 {
		t.Fatalf("expected no preview and no chafa run for a markdown file, got %q after %v", got, runner.Calls)
	}
}

func TestRenderSkipsWithoutChafa(t *testing.T) {
	runner := &FakeChafaRunner{Stdout: []byte("art")}
	if got := newTestPreviewer(runner, missingFromPath).Render(context.Background(), "/a.png"); got != "" || len(runner.Calls) != 0 {
		t.Fatalf("expected no preview when chafa is missing, got %q after %v", got, runner.Calls)
	}
}

func TestRenderSkipsWhenChafaFails(t *testing.T) {
	runner := &FakeChafaRunner{Stdout: []byte("partial"), Err: errors.New("exit status 1")}
	if got := newTestPreviewer(runner, foundOnPath).Render(context.Background(), "/a.jpg"); got != "" {
		t.Fatalf("expected no preview when chafa fails, got %q", got)
	}
}

func TestExecRunnerReportsFailure(t *testing.T) {
	_, err := ExecRunner{}.Output(context.Background(), "/nonexistent/cade-test-program")
	if err == nil {
		t.Fatal("expected an error running a missing program")
	}
}
