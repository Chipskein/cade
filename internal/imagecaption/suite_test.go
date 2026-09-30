package imagecaption

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testfakes"
)

const shippedCaptionSuite = "../../testdata/queries/captions.json"

func TestScoreFindsWordsIgnoringCaseAndAccentsAndReportsLeaks(t *testing.T) {
	c := CaptionCase{Image: "a.png", MustContain: []string{"Coleta", "4821", "Curitiba"}, Secrets: []string{"ghp_abc"}}
	score := c.Score("Pedido 4821, aguardando COLETA; token ghp_abc")
	if score.Found != 2 || len(score.Missing) != 1 || score.Missing[0] != "Curitiba" || len(score.Leaked) != 1 {
		t.Fatalf("unexpected score %+v", score)
	}
}

func TestCoverageIsTheShareOfWordsFound(t *testing.T) {
	suite := CaptionSuite{Cases: []CaptionCase{{MustContain: []string{"a", "b"}}, {MustContain: []string{"c", "d"}}}}
	if got := suite.Coverage([]CaptionScore{{Found: 2}, {Found: 1}}); got != 0.75 {
		t.Fatalf("expected 3 of 4 words, got %v", got)
	}
}

func TestLoadCaptionSuiteRejectsAMissingFloor(t *testing.T) {
	if _, err := LoadCaptionSuite(strings.NewReader(`{"cases": [{"image": "a.png"}]}`)); err == nil {
		t.Fatal("expected a suite without minimum_coverage to be rejected")
	}
}

// Every case names a fixture that exists, and every fixture has a case.
func TestShippedCaptionSuiteMatchesTheFixtures(t *testing.T) {
	file, err := os.Open(shippedCaptionSuite)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	suite, err := LoadCaptionSuite(file)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, _ := filepath.Glob("../../testdata/images/*.*")
	images := 0
	for _, path := range fixtures {
		images += boolCount(!strings.HasSuffix(path, ".go"))
	}
	for _, c := range suite.Cases {
		if _, err := os.Stat(filepath.Join("../../testdata/images", c.Image)); err != nil || len(c.MustContain) == 0 {
			t.Errorf("case %q: fixture missing or no words required (%v)", c.Image, err)
		}
	}
	if images != len(suite.Cases) {
		t.Fatalf("expected one case per fixture image, got %d cases for %d images", len(suite.Cases), images)
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestFixtureCaptionsReuseTheCacheAcrossRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "captions.json")
	describer := &testfakes.FakeImageDescriber{Reply: fakeReply}
	load := func() (ClosableDescriber, error) { return describer, nil }
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	raw := pngFile(t, 4, 4, 1).Data
	first, _ := OpenFixtureCaptions(path, load, "m.gguf", logger)
	if _, err := first.Describe(context.Background(), "a.png", raw); err != nil || first.Save() != nil {
		t.Fatalf("describe and save: %v", err)
	}
	second, err := OpenFixtureCaptions(path, load, "m.gguf", logger)
	if err != nil {
		t.Fatal(err)
	}
	image, err := second.Describe(context.Background(), "a.png", raw)
	if err != nil || image.Description != "um terminal" || len(describer.Images) != 1 {
		t.Fatalf("expected the cached description without the model, got %+v after %d descriptions", image, len(describer.Images))
	}
	second.Refresh = true
	if _, err := second.Describe(context.Background(), "a.png", raw); err != nil || len(describer.Images) != 2 {
		t.Fatalf("expected Refresh to describe again, got %d descriptions", len(describer.Images))
	}
}
