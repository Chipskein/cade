package devtasks

import (
	"io"
	"os"
	"slices"
)

// testModelVar names the variable a model-backed test reads a model path from.
type testModelVar string

const (
	testEmbeddingModel  testModelVar = "CADE_TEST_EMBEDDING_MODEL"
	testGenerationModel testModelVar = "CADE_TEST_GENERATION_MODEL"
	testVisionProjector testModelVar = "CADE_TEST_VISION_PROJECTOR"
	testRerankerModel   testModelVar = "CADE_TEST_RERANKER_MODEL"
)

var testModelVars = map[Model]testModelVar{
	EmbeddingModel: testEmbeddingModel, GenerationModel: testGenerationModel,
	VisionProjector: testVisionProjector, RerankerModel: testRerankerModel,
}

// Variables the retrieval suite reads (internal/retrievalsuite).
const (
	evalModeVar  = "CADE_EVAL_MODE"
	evalScaleVar = "CADE_EVAL_SCALE"
	evalTopKVar  = "CADE_EVAL_TOP_K"
	// The scale curve outgrows EVAL_TIMEOUT's default at 10k events on CPU.
	scaleTimeout = "3h"
	benchTimeout = "2h"
)

// modelRun is one `go test` against real models.
type modelRun struct {
	models []Model
	// env holds KEY=VALUE pairs besides the model paths.
	env    []string
	args   []string
	stdout io.Writer
}

// TestModels runs every test, including the llama.cpp binding against the
// real models, on the CPU build.
func (t *Tasks) TestModels() error {
	if err := t.EnsureLlama(LlamaCPU); err != nil {
		return err
	}
	if err := t.EnsureModels(runtimeModels...); err != nil {
		return err
	}
	return t.runner.Run(t.modelTestCommand(modelRun{models: runtimeModels, args: []string{"test", "-tags", fts5Tag, allPackages}}))
}

// imageModels describe the image fixtures; the suites whose corpus has
// images get their paths, and load them only for a fixture missing from
// the cache evalCaptions writes (phase 19).
var imageModels = []Model{GenerationModel, VisionProjector}

// withImageModels adds the image models to models.
func withImageModels(models ...Model) []Model {
	return append(models, imageModels...)
}

// EvalCaptions describes the image fixtures against
// testdata/queries/captions.json and caches the descriptions.
func (t *Tasks) EvalCaptions() error {
	return t.runEval(modelRun{models: imageModels, args: t.evalTestArgs(t.settings.EvalTimeout, "TestCaptionSuiteWithModel", "./internal/imagecaption")})
}

// EvalPlan scores the question planner against testdata/queries/plan.json.
func (t *Tasks) EvalPlan() error {
	return t.runEval(modelRun{models: []Model{GenerationModel}, args: t.evalTestArgs(t.settings.EvalTimeout, "TestPlanSuiteWithModel", "./internal/queryplan")})
}

// EvalRetrieval scores retrieval with the real embedder and SQLite store,
// then times a cold ask per top_k.
func (t *Tasks) EvalRetrieval() error {
	suite := modelRun{models: withImageModels(EmbeddingModel), env: []string{evalModeVar + "=" + t.settings.EvalMode},
		args: t.evalTestArgs(t.settings.EvalTimeout, "TestRetrieval(Calibration|Suite|Sweep)WithModel", "./internal/retrievalsuite")}
	if err := t.runEval(suite); err != nil {
		return err
	}
	return t.runEval(modelRun{models: []Model{EmbeddingModel, GenerationModel}, args: []string{
		"test", "-tags", t.evalTags(), "-run", "^$", "-bench", "^BenchmarkColdAskTopK$", "-benchtime=1x",
		"-timeout", t.settings.EvalTimeout, "./internal/benchmarks",
	}})
}

// EvalInjection answers the prompt-injection cases with both real models.
func (t *Tasks) EvalInjection() error {
	return t.runEval(modelRun{models: withImageModels(EmbeddingModel),
		args: t.evalTestArgs(t.settings.EvalTimeout, "TestInjectionWithModel", "./internal/retrievalsuite")})
}

// EvalScale writes the test-set metrics as the corpus grows to SCALE_REPORT
// and the terminal.
func (t *Tasks) EvalScale() error {
	run := modelRun{models: withImageModels(EmbeddingModel),
		env:  []string{evalScaleVar + "=" + t.settings.Scale, evalTopKVar + "=" + t.settings.ScaleTopK, evalModeVar + "=" + t.settings.EvalMode},
		args: t.evalTestArgs(scaleTimeout, "TestRetrievalScaleWithModel", "./internal/retrievalsuite")}
	if err := t.prepareEval(run.models); err != nil {
		return err
	}
	// Created only now, so a failed download leaves the previous report.
	report, err := os.Create(t.path(t.settings.ScaleReport))
	if err != nil {
		return err
	}
	defer report.Close()
	run.stdout = io.MultiWriter(os.Stdout, report)
	return t.runner.Run(t.modelTestCommand(run))
}

// EvalRerank measures the test set with and without reranking (phase 17).
func (t *Tasks) EvalRerank() error {
	return t.runEval(modelRun{models: withImageModels(EmbeddingModel, RerankerModel), env: []string{evalScaleVar + "=" + t.settings.Scale},
		args: t.evalTestArgs(t.settings.EvalTimeout, "TestRetrievalRerankWithModel", "./internal/retrievalsuite")})
}

// Bench measures latency and memory of storage, the models and a whole ask.
func (t *Tasks) Bench() error {
	return t.runEval(modelRun{models: runtimeModels, args: []string{
		"test", "-tags", joinTags(t.evalTags(), dbstatTag), "-run", "^$", "-bench", ".", "-benchtime", "5x", "-timeout", benchTimeout,
		"./internal/storage/sqlitestore", "./internal/benchmarks",
	}})
}

func (t *Tasks) evalTestArgs(timeout, testPattern, pkg string) []string {
	return []string{"test", "-tags", t.evalTags(), "-count=1", "-v", "-timeout", timeout, "-run", testPattern, pkg}
}

func (t *Tasks) evalTags() string {
	return joinTags(append([]string{fts5Tag}, t.settings.ExtraGoTags...)...)
}

func (t *Tasks) runEval(run modelRun) error {
	if err := t.prepareEval(run.models); err != nil {
		return err
	}
	return t.runner.Run(t.modelTestCommand(run))
}

// prepareEval builds llama.cpp for GPU when the tags ask for it, and
// downloads the models.
func (t *Tasks) prepareEval(models []Model) error {
	variant := LlamaCPU
	if slices.Contains(t.settings.ExtraGoTags, cudaTag) {
		variant = LlamaCUDA
	}
	if err := t.EnsureLlama(variant); err != nil {
		return err
	}
	return t.EnsureModels(models...)
}

func (t *Tasks) modelTestCommand(run modelRun) Command {
	env := append([]string{}, run.env...)
	for _, model := range run.models {
		env = append(env, string(testModelVars[model])+"="+t.settings.ModelPath(model))
	}
	return Command{Name: "go", Args: run.args, Env: env, Stdout: run.stdout}
}
