package devtasks

import (
	"strings"
	"testing"
)

const stampedLinkerFlags = "'-X github.com/chipskein/cade/internal/buildinfo.version=v1.2.3 -X github.com/chipskein/cade/internal/buildinfo.commit=abc " +
	"-X github.com/chipskein/cade/internal/buildinfo.date=2026-09-28 -X github.com/chipskein/cade/internal/buildinfo.llamaTag=b11195'"

func TestBuildStampsTheVersionFromGit(t *testing.T) {
	world := newTestWorld(t)
	world.runner.Outputs["git describe --tags --always --dirty"] = "v1.2.3"
	world.runner.Outputs["git rev-parse --short=12 HEAD"] = "abc"
	world.runner.Outputs["git log -1 --format=%cd --date=format:%Y-%m-%d"] = "2026-09-28"
	if err := world.tasks().BuildCPU(); err != nil {
		t.Fatal(err)
	}
	lines := world.runner.Lines()
	want := "go build -tags sqlite_fts5 -ldflags " + stampedLinkerFlags + " -o bin/cade ./cmd/cade"
	if lines[len(lines)-1] != want {
		t.Errorf("build = %q\nwant %q", lines[len(lines)-1], want)
	}
}

func TestBuildCUDAPrefersTheVariablesOverGit(t *testing.T) {
	world := newTestWorld(t)
	world.writeFile(t, cudaBuildDir+"/"+lastLlamaLibrary, "")
	world.env[EnvVersion], world.env[EnvCommit], world.env[EnvBuildDate] = "v1.2.3", "abc", "2026-09-28"
	if err := world.tasks().BuildCUDA(); err != nil {
		t.Fatal(err)
	}
	assertLines(t, world.runner.Lines(), "go build -tags sqlite_fts5,cuda -ldflags "+stampedLinkerFlags+" -o bin/cade ./cmd/cade")
}

func TestBuildStampWithoutGitReportsDev(t *testing.T) {
	world := newTestWorld(t)
	world.runner.FailOn = "git"
	if stamp := world.tasks().resolveBuildStamp(); stamp != (buildStamp{version: "dev"}) {
		t.Errorf("stamp = %+v, want version dev only", stamp)
	}
}

func TestTestVetLintAndCoverRunTheGoTools(t *testing.T) {
	world := newTestWorld(t)
	tasks := world.tasks()
	for _, run := range []func() error{tasks.Test, tasks.Vet, tasks.Lint, tasks.Cover} {
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	assertLines(t, world.runner.Lines(), "go test -tags sqlite_fts5 ./...", "go vet -tags sqlite_fts5 ./...", "golangci-lint run ./...",
		"go test -tags sqlite_fts5 -coverprofile=coverage.out ./...", "go tool cover -func=coverage.out")
}

func TestFormatRewritesEveryGoDirectory(t *testing.T) {
	world := newTestWorld(t)
	if err := world.tasks().Format(); err != nil {
		t.Fatal(err)
	}
	assertLines(t, world.runner.Lines(), "gofmt -w cmd internal magefiles")
}

func TestFormatCheckListsUnformattedFiles(t *testing.T) {
	world := newTestWorld(t)
	world.runner.Outputs["gofmt -l cmd internal magefiles"] = "internal/a.go"
	err := world.tasks().FormatCheck()
	if err == nil || !strings.Contains(err.Error(), "internal/a.go") {
		t.Errorf("err = %v, want it to list internal/a.go", err)
	}
	world.runner.Outputs["gofmt -l cmd internal magefiles"] = ""
	if err := world.tasks().FormatCheck(); err != nil {
		t.Errorf("formatted tree: %v", err)
	}
}
