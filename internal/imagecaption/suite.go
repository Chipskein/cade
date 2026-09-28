package imagecaption

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/textnorm"
)

// CaptionCase is one fixture image of testdata/images and what its stored
// text must hold: MustContain words (a description or a transcription
// without them would not be found by them), and none of Secrets once
// masked.
type CaptionCase struct {
	Image       string   `json:"image"`
	MustContain []string `json:"must_contain"`
	Secrets     []string `json:"secrets"`
}

// CaptionSuite is testdata/queries/captions.json; MinimumCoverage is the
// floor for the share of MustContain words found over all cases.
type CaptionSuite struct {
	MinimumCoverage float64       `json:"minimum_coverage"`
	Cases           []CaptionCase `json:"cases"`
}

// LoadCaptionSuite reads a caption suite.
//
//	suite, err := imagecaption.LoadCaptionSuite(file)
func LoadCaptionSuite(reader io.Reader) (CaptionSuite, error) {
	var suite CaptionSuite
	if err := json.NewDecoder(reader).Decode(&suite); err != nil {
		return CaptionSuite{}, fmt.Errorf("decode caption suite, expected {minimum_coverage, cases: [{image, must_contain, secrets}]}: %w", err)
	}
	if len(suite.Cases) == 0 || suite.MinimumCoverage <= 0 || suite.MinimumCoverage > 1 {
		return CaptionSuite{}, fmt.Errorf("caption suite has %d cases and minimum_coverage %v; expected cases and a floor in (0, 1]",
			len(suite.Cases), suite.MinimumCoverage)
	}
	return suite, nil
}

// CaptionScore is how one case's stored text fared.
type CaptionScore struct {
	Image   string
	Found   int
	Missing []string
	Leaked  []string
}

// Score checks storedText, the text as the pipeline stores it (masked),
// ignoring case and accents.
func (c CaptionCase) Score(storedText string) CaptionScore {
	folded := textnorm.Fold(storedText)
	score := CaptionScore{Image: c.Image}
	for _, word := range c.MustContain {
		if strings.Contains(folded, textnorm.Fold(word)) {
			score.Found++
			continue
		}
		score.Missing = append(score.Missing, word)
	}
	for _, secret := range c.Secrets {
		if strings.Contains(storedText, secret) {
			score.Leaked = append(score.Leaked, secret)
		}
	}
	return score
}

// Coverage is the share of required words found over all scores.
func (s CaptionSuite) Coverage(scores []CaptionScore) float64 {
	required, found := 0, 0
	for i, score := range scores {
		required += len(s.Cases[i].MustContain)
		found += score.Found
	}
	if required == 0 {
		return 1
	}
	return float64(found) / float64(required)
}
