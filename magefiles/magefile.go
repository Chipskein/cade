//go:build mage

// Development tasks for cade. Run them with `go tool mage <target>`, list
// them with `go tool mage -l`; settings are environment variables, e.g.
// `GO_TAGS= go tool mage bench`. The logic lives in internal/devtasks.
package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/magefile/mage/mg"

	"github.com/chipskein/cade/internal/devtasks"
)

// repoRoot is where mage runs targets: the directory above magefiles/.
const repoRoot = "."

// Default is the target of a bare `go tool mage`.
var Default = Build

var tasks = sync.OnceValue(func() *devtasks.Tasks {
	return devtasks.NewTasks(devtasks.Dependencies{
		Root:     repoRoot,
		Runner:   devtasks.ExecRunner{Dir: repoRoot, Echo: os.Stderr},
		Fetcher:  devtasks.HTTPFetcher{Client: http.DefaultClient},
		Env:      devtasks.ProcessEnvironment{},
		Progress: os.Stderr,
	})
})

// Build compiles the CPU binary into bin/cade.
func Build() error { return tasks().Build() }

// Cuda compiles the NVIDIA binary into bin/cade (CUDA Toolkit required).
func Cuda() error { return tasks().BuildCUDA() }

// Install copies bin/cade (CPU or CUDA) to $DESTDIR$PREFIX/bin, building the CPU one if missing.
func Install() error { return tasks().Install() }

// Uninstall removes $DESTDIR$PREFIX/bin/cade.
func Uninstall() error { return tasks().Uninstall() }

// Dist builds the release archive and its SHA-256 in dist/.
func Dist() error { return tasks().Dist() }

// Test runs every unit test.
func Test() error { return tasks().Test() }

// Cover runs the tests with a coverage profile and prints the per-function table.
func Cover() error { return tasks().Cover() }

// Fuzz runs each Teams cache parser fuzz target for $FUZZTIME (default 30s).
func Fuzz() error { return tasks().Fuzz() }

// TestModels runs every test, including the llama.cpp binding against the real models.
func TestModels() error { return tasks().TestModels() }

// Eval runs the plan, retrieval and injection suites with the real models.
func Eval() { mg.SerialDeps(EvalPlan, EvalRetrieval, EvalInjection) }

// EvalPlan scores the question planner against testdata/queries/plan.json.
func EvalPlan() error { return tasks().EvalPlan() }

// EvalRetrieval scores retrieval (testdata/queries/retrieval/); $MODE compares search modes.
func EvalRetrieval() error { return tasks().EvalRetrieval() }

// EvalInjection answers the prompt-injection cases (testdata/queries/injection.json).
func EvalInjection() error { return tasks().EvalInjection() }

// EvalScale writes test-set metrics at each $SCALE corpus size to $SCALE_REPORT.
func EvalScale() error { return tasks().EvalScale() }

// EvalRerank measures retrieval with and without reranking (phase 17; not part of eval).
func EvalRerank() error { return tasks().EvalRerank() }

// Bench measures latency and memory; GO_TAGS= measures the CPU build.
func Bench() error { return tasks().Bench() }

// Fmt rewrites the sources with gofmt.
func Fmt() error { return tasks().Format() }

// FmtCheck fails listing the files gofmt would change.
func FmtCheck() error { return tasks().FormatCheck() }

// Vet runs go vet.
func Vet() error { return tasks().Vet() }

// Lint runs the pinned golangci-lint (see .golangci.yml).
func Lint() error { return tasks().Lint() }

// Check runs what CI runs on every push: fmtCheck, vet, lint and test.
func Check() { mg.SerialDeps(FmtCheck, Vet, Lint, Test) }

// Llama clones the pinned llama.cpp and builds its CPU libraries.
func Llama() error { return tasks().EnsureLlama(devtasks.LlamaCPU) }

// LlamaCuda builds the CUDA llama.cpp libraries.
func LlamaCuda() error { return tasks().EnsureLlama(devtasks.LlamaCUDA) }

// Models downloads the models to $MODELS_DIR (default ~/.local/share/cade/models).
func Models() error { return tasks().Models() }

// Clean removes bin/, dist/, coverage.out and the llama.cpp builds.
func Clean() error { return tasks().Clean() }

// Print prints a pinned value for CI cache keys: `go tool mage print LLAMA_TAG`
func Print(name string) error {
	value, err := devtasks.PinnedValue(name)
	if err != nil {
		return err
	}
	fmt.Println(value)
	return nil
}
