package queryplan

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/llm/llamacpp"
)

// generationModelEnv points at the real GGUF model (see the mage targets
// testModels and evalPlan); without it the suite is skipped.
const generationModelEnv = "CADE_TEST_GENERATION_MODEL"

// TestPlanSuiteWithModel runs the shipped suite against the real model and
// fails when a field drops below the suite's minimum_accuracy.
func TestPlanSuiteWithModel(t *testing.T) {
	path := os.Getenv(generationModelEnv)
	if path == "" {
		t.Skipf("%s not set; skipping the plan suite", generationModelEnv)
	}
	generator, err := llamacpp.LoadGenerator(llamacpp.ModelOptions{Path: path, ContextTokens: 4096, GPULayers: -1})
	if err != nil {
		t.Fatalf("load generator: %v", err)
	}
	defer generator.Close()
	suite := loadShippedSuite(t)
	board, err := RunSuite(context.Background(), NewPlanner(generator), suite, logCase(t))
	if err != nil {
		t.Fatal(err)
	}
	var report strings.Builder
	board.WriteReport(&report)
	t.Log("\n" + report.String())
	if below := board.BelowMinimum(suite.MinimumAccuracy); len(below) > 0 {
		t.Fatalf("fields below minimum_accuracy: %v", below)
	}
}

// logCase prints each question as soon as it is scored; go test -v streams
// t.Logf, so a slow run shows progress.
func logCase(t *testing.T) CaseScored {
	return func(done, total int, result CaseResult) {
		mark := "ok"
		if len(result.Mismatches) > 0 {
			mark = "✗ "
		}
		t.Logf("[%2d/%d] %s %s", done, total, mark, result.Question)
	}
}
