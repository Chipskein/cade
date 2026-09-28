package devtasks

import (
	"os"
	"strings"
	"testing"
)

func newEvalWorld(t *testing.T) (*testWorld, *Tasks) {
	t.Helper()
	world := newTestWorld(t)
	world.env[EnvGoTags] = ""
	world.env[EnvModelsDir] = "/models"
	for _, source := range modelSources {
		world.env[source.pathVar] = world.writeFile(t, "models/"+source.fileName, "gguf")
	}
	return world, world.tasks()
}

func TestEvalPlanPassesTheGenerationModel(t *testing.T) {
	world, tasks := newEvalWorld(t)
	if err := tasks.EvalPlan(); err != nil {
		t.Fatal(err)
	}
	command := world.runner.Commands[0]
	if command.Env[0] != "CADE_TEST_GENERATION_MODEL="+tasks.Settings().ModelPath(GenerationModel) {
		t.Errorf("env = %q", command.Env)
	}
	assertLines(t, []string{strings.Join(command.Args, " ")}, "test -tags sqlite_fts5 -count=1 -v -timeout 1h -run TestPlanSuiteWithModel ./internal/queryplan")
}

func TestEvalWithCUDATagsNeedsTheCUDABuild(t *testing.T) {
	world, _ := newEvalWorld(t)
	world.env[EnvGoTags] = "cuda"
	err := world.tasks().EvalInjection()
	if err == nil || !strings.Contains(err.Error(), "nvcc") {
		t.Errorf("err = %v, want the CUDA build to be required", err)
	}
}

func TestEvalRetrievalRunsTheSuiteThenTheColdAsk(t *testing.T) {
	world, tasks := newEvalWorld(t)
	if err := tasks.EvalRetrieval(); err != nil {
		t.Fatal(err)
	}
	if len(world.runner.Commands) != 2 || !strings.Contains(world.runner.Lines()[1], "-bench ^BenchmarkColdAskTopK$") {
		t.Errorf("commands = %q", world.runner.Lines())
	}
	if world.runner.Commands[0].Env[0] != "CADE_EVAL_MODE=" {
		t.Errorf("suite env = %q, want CADE_EVAL_MODE first", world.runner.Commands[0].Env)
	}
}

func TestEvalScaleWritesTheReport(t *testing.T) {
	world, tasks := newEvalWorld(t)
	world.writeFile(t, tasks.Settings().ScaleReport, "old")
	if err := tasks.EvalScale(); err != nil {
		t.Fatal(err)
	}
	command := world.runner.Commands[0]
	if !strings.Contains(command.String(), "CADE_EVAL_SCALE=1000,10000 CADE_EVAL_TOP_K=6") || command.Stdout == nil {
		t.Errorf("command = %q (stdout %v)", command.String(), command.Stdout)
	}
	if report, _ := os.ReadFile(world.root + "/" + tasks.Settings().ScaleReport); len(report) != 0 {
		t.Errorf("report = %q, want it truncated for the new run", report)
	}
}

func TestEvalRerankInjectionBenchAndTestModelsUseTheirModels(t *testing.T) {
	world, tasks := newEvalWorld(t)
	for _, run := range []func() error{tasks.EvalRerank, tasks.EvalInjection, tasks.Bench, tasks.TestModels} {
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"CADE_TEST_RERANKER_MODEL", "TestInjectionWithModel", "-bench . -benchtime 5x", "CADE_TEST_VISION_PROJECTOR"}
	for i, fragment := range want {
		if !strings.Contains(world.runner.Lines()[i], fragment) {
			t.Errorf("command %d = %q, want %q in it", i, world.runner.Lines()[i], fragment)
		}
	}
}

func TestEvalCaptionsDescribesWithTheGeneratorAndProjector(t *testing.T) {
	world, tasks := newEvalWorld(t)
	if err := tasks.EvalCaptions(); err != nil {
		t.Fatal(err)
	}
	line := world.runner.Lines()[0]
	for _, fragment := range []string{"CADE_TEST_GENERATION_MODEL=", "CADE_TEST_VISION_PROJECTOR=", "-run TestCaptionSuiteWithModel ./internal/imagecaption"} {
		if !strings.Contains(line, fragment) {
			t.Errorf("command = %q, want %q in it", line, fragment)
		}
	}
}

// The corpus has images: without a cached description, the suites
// describe them, so they get the image models too.
func TestSuitesOverTheCorpusGetTheImageModels(t *testing.T) {
	world, tasks := newEvalWorld(t)
	if err := tasks.EvalInjection(); err != nil {
		t.Fatal(err)
	}
	if line := world.runner.Lines()[0]; !strings.Contains(line, "CADE_TEST_VISION_PROJECTOR=") || !strings.Contains(line, "CADE_TEST_EMBEDDING_MODEL=") {
		t.Errorf("command = %q, want the embedder and the image models", line)
	}
}
