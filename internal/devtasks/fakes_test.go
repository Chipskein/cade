package devtasks

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FakeCommandRunner records the commands it is given, answers Output from
// Outputs (keyed by Command.String) and fails any command containing FailOn.
type FakeCommandRunner struct {
	Outputs  map[string]string
	FailOn   string
	Commands []Command
}

func (f *FakeCommandRunner) Run(command Command) error {
	f.Commands = append(f.Commands, command)
	return f.failure(command)
}

func (f *FakeCommandRunner) Output(command Command) (string, error) {
	f.Commands = append(f.Commands, command)
	if err := f.failure(command); err != nil {
		return "", err
	}
	return f.Outputs[command.String()], nil
}

func (f *FakeCommandRunner) failure(command Command) error {
	if f.FailOn != "" && strings.Contains(command.String(), f.FailOn) {
		return errors.New("exit status 1")
	}
	return nil
}

// Lines returns each recorded command line.
func (f *FakeCommandRunner) Lines() []string {
	lines := make([]string, 0, len(f.Commands))
	for _, command := range f.Commands {
		lines = append(lines, command.String())
	}
	return lines
}

// FakeEnvironment is a fixed set of variables.
type FakeEnvironment map[EnvVar]string

func (f FakeEnvironment) LookupEnv(name EnvVar) (string, bool) {
	value, found := f[name]
	return value, found
}

// FakeModelFetcher writes "<url>" as the body of every download, or fails
// with FailWith after writing part of it.
type FakeModelFetcher struct {
	FailWith error
	URLs     []string
}

func (f *FakeModelFetcher) Fetch(url string, destination io.Writer) error {
	f.URLs = append(f.URLs, url)
	if _, err := io.WriteString(destination, url); err != nil {
		return err
	}
	return f.FailWith
}

// testWorld is a repository root in a temporary directory with llama.cpp
// already built, so tasks go straight to the command under test.
type testWorld struct {
	root     string
	runner   *FakeCommandRunner
	fetcher  *FakeModelFetcher
	progress *bytes.Buffer
	env      FakeEnvironment
}

func newTestWorld(t *testing.T) *testWorld {
	t.Helper()
	root := t.TempDir()
	world := &testWorld{root: root, runner: &FakeCommandRunner{Outputs: map[string]string{}}, fetcher: &FakeModelFetcher{},
		progress: &bytes.Buffer{}, env: FakeEnvironment{EnvHome: filepath.Join(root, "home"), EnvCudaHome: filepath.Join(root, "cuda")}}
	world.writeFile(t, cpuBuildDir+"/"+lastLlamaLibrary, "")
	return world
}

func (w *testWorld) tasks() *Tasks {
	return NewTasks(Dependencies{Root: w.root, Runner: w.runner, Fetcher: w.fetcher, Env: w.env, Progress: w.progress})
}

func (w *testWorld) writeFile(t *testing.T, relative, content string) string {
	t.Helper()
	path := filepath.Join(w.root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertLines(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("commands:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
