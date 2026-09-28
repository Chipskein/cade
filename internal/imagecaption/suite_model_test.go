package imagecaption_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/evalimages"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/privacy"
)

// The suite and its fixtures; the model paths come from the variables of
// the mage target evalCaptions, and without them the suite is skipped.
const (
	shippedSuitePath = "../../testdata/queries/captions.json"
	fixtureDirectory = "../../testdata/images"
)

// TestCaptionSuiteWithModel describes every fixture with the real model,
// masks the result as ingestion stores it, and checks the words and the
// secrets of testdata/queries/captions.json. The descriptions are cached
// for the retrieval and injection suites.
func TestCaptionSuiteWithModel(t *testing.T) {
	suite := loadShippedCaptionSuite(t)
	captions := openFixtureCaptions(t)
	captions.Refresh = true
	var scores []imagecaption.CaptionScore
	for _, c := range suite.Cases {
		stored := storedFixture(t, captions, c.Image)
		score := c.Score(stored)
		t.Logf("%s\n    %s\n    faltando: %v", c.Image, strings.ReplaceAll(stored, "\n", "\n    "), score.Missing)
		if len(score.Leaked) > 0 {
			t.Errorf("%s: secrets stored after masking: %v", c.Image, score.Leaked)
		}
		scores = append(scores, score)
	}
	if err := captions.Save(); err != nil {
		t.Fatal(err)
	}
	coverage := suite.Coverage(scores)
	t.Logf("cobertura: %.2f (mínimo %.2f)", coverage, suite.MinimumCoverage)
	if coverage < suite.MinimumCoverage {
		t.Fatalf("caption coverage %.2f is below the suite floor %.2f", coverage, suite.MinimumCoverage)
	}
}

func loadShippedCaptionSuite(t *testing.T) imagecaption.CaptionSuite {
	t.Helper()
	file, err := os.Open(shippedSuitePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	suite, err := imagecaption.LoadCaptionSuite(file)
	if err != nil {
		t.Fatal(err)
	}
	return suite
}

func openFixtureCaptions(t *testing.T) *imagecaption.FixtureCaptions {
	t.Helper()
	if os.Getenv(evalimages.GenerationModelEnv) == "" || os.Getenv(evalimages.VisionProjectorEnv) == "" {
		t.Skipf("%s or %s not set; skipping the caption suite", evalimages.GenerationModelEnv, evalimages.VisionProjectorEnv)
	}
	captions, err := evalimages.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { captions.Close() })
	return captions
}

// storedFixture is what ingestion stores for the fixture, masked: its
// text without the file name (a word in the name proves nothing about the
// description) and its description fields.
func storedFixture(t *testing.T, captions *imagecaption.FixtureCaptions, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	image, err := captions.Describe(context.Background(), name, raw)
	if err != nil {
		t.Fatal(err)
	}
	ev := privacy.Event(event.Event{Source: event.SourceFile, Content: filesource.EventContent(name, image.SearchableText()), Metadata: image.Metadata()}, true)
	masked := ev.Image()
	_, text, _ := strings.Cut(ev.Content, "\n")
	return text + "\n" + masked.Description + "\n" + masked.VisibleText
}
