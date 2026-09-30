package devtasks

import (
	"io"
	"path/filepath"
	"strings"
)

// Pinned versions: bumping one is a reviewed change here.
const (
	LlamaTag = "b11195"
	// GolangciLintVersion is pinned so `go tool mage lint` and CI agree; CI
	// builds it with this module's Go, since a golangci-lint built with an
	// older Go refuses newer modules.
	GolangciLintVersion = "v2.14.0"
)

// Paths relative to the repository root, and the Go build inputs.
const (
	llamaDir         = "third_party/llama.cpp"
	llamaRepoURL     = "https://github.com/ggml-org/llama.cpp"
	binaryPath       = "bin/cade"
	binDir           = "bin"
	mainPackage      = "./cmd/cade"
	allPackages      = "./..."
	distDir          = "dist"
	coverageProfile  = "coverage.out"
	buildInfoPackage = "github.com/chipskein/cade/internal/buildinfo"
	// The SQLite driver only compiles FTS5 (keyword search) with this tag;
	// every build and test needs it.
	fts5Tag = "sqlite_fts5"
	// dbstat (bytes per table) is only needed by the storage benchmarks (#40),
	// so only `mage bench` compiles it in.
	dbstatTag = "sqlite_dbstat"
	cudaTag   = "cuda"
)

// Dependencies are what Tasks needs from the outside world.
type Dependencies struct {
	// Root is the repository root; relative paths resolve against it.
	Root    string
	Runner  CommandRunner
	Fetcher ModelFetcher
	Env     Environment
	// Progress receives lines such as the fuzz target being run.
	Progress io.Writer
}

// Tasks runs the development tasks the magefile exposes.
type Tasks struct {
	root     string
	runner   CommandRunner
	fetcher  ModelFetcher
	env      Environment
	progress io.Writer
	settings Settings
}

// NewTasks loads the settings from deps.Env.
//
//	tasks := NewTasks(Dependencies{Root: ".", Runner: ExecRunner{Dir: "."}, ...})
func NewTasks(deps Dependencies) *Tasks {
	return &Tasks{
		root: deps.Root, runner: deps.Runner, fetcher: deps.Fetcher, env: deps.Env,
		progress: deps.Progress, settings: LoadSettings(deps.Env),
	}
}

// Settings returns the settings in effect.
func (t *Tasks) Settings() Settings {
	return t.settings
}

func (t *Tasks) path(relative string) string {
	return filepath.Join(t.root, relative)
}

func joinTags(tags ...string) string {
	return strings.Join(tags, goTagSeparator)
}
