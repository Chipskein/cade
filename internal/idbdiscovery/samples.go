package idbdiscovery

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// maxSampleRunes truncates sample text: enough to tell a name from an id
// or an HTML body from plain text, not enough to carry a message.
const maxSampleRunes = 40

// Sample renderings for values whose content the model does not need.
const (
	sampleDate     = "<date>"
	sampleBinary   = "<binary>"
	sampleFraction = "<n:fraction>"
	sampleTruncate = "…"
)

// collectSamples renders up to limit distinct samples of path from
// records. Containers have none: their paths speak for them.
func collectSamples(path idbmap.Path, records []webstore.Record, limit int) []string {
	compiled, err := idbmap.CompilePath(path)
	if err != nil {
		return nil
	}
	var samples []string
	for _, record := range records {
		if samples = appendSamples(samples, compiled.Resolve(record.Value), limit); len(samples) >= limit {
			return samples
		}
	}
	return samples
}

func appendSamples(samples []string, values []*v8value.Value, limit int) []string {
	for _, value := range values {
		sample, ok := renderSample(value)
		if ok && len(samples) < limit && !slices.Contains(samples, sample) {
			samples = append(samples, sample)
		}
	}
	return samples
}

// renderSample shows a value without its content where the content is
// not needed: a number only as its digit count ("<n:13>" tells epoch
// milliseconds from seconds), text masked like the summary and truncated.
func renderSample(value *v8value.Value) (string, bool) {
	switch value.Kind {
	case v8value.KindString:
		return maskText(value.Text), true
	case v8value.KindNumber, v8value.KindBigInt:
		return numberShape(value), true
	case v8value.KindBool:
		return fmt.Sprint(value.Bool), true
	case v8value.KindDate:
		return sampleDate, true
	case v8value.KindBinary:
		return sampleBinary, true
	}
	return "", false
}

func maskText(text string) string {
	masked := strings.Join(strings.Fields(idbschema.MaskName(text)), " ")
	if runes := []rune(masked); len(runes) > maxSampleRunes {
		return string(runes[:maxSampleRunes]) + sampleTruncate
	}
	return masked
}

// numberShape is "<n:13>" for a 13-digit integer and "<n:fraction>" for
// a non-integer.
func numberShape(value *v8value.Value) string {
	if value.Kind == v8value.KindNumber && value.Number != float64(int64(value.Number)) {
		return sampleFraction
	}
	digits := strings.TrimPrefix(value.Text, "-")
	if value.Kind == v8value.KindNumber {
		digits = fmt.Sprint(max(int64(value.Number), -int64(value.Number)))
	}
	return fmt.Sprintf("<n:%d>", len(digits))
}
