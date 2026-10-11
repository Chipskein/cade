package idbmap

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/htmltext"
	"github.com/chipskein/cade/internal/v8value"
)

// Transform is the closed set of conversions a field may apply; the
// language grows only when a real application needs another.
type Transform string

const (
	TransformNone     Transform = ""
	TransformHTMLText Transform = "html_text"
	TransformTrim     Transform = "trim"
	TransformUnixMS   Transform = "unix_ms"
	TransformUnixS    Transform = "unix_s"
	TransformISO8601  Transform = "iso8601"
)

// transformsByShape lists what each kind of field accepts. A time field
// with no transform reads a JavaScript Date.
var transformsByShape = map[fieldShape][]Transform{
	shapeText: {TransformNone, TransformHTMLText, TransformTrim},
	shapeTime: {TransformNone, TransformUnixMS, TransformUnixS, TransformISO8601},
	shapeFlag: {TransformNone},
}

func validateTransform(shape fieldShape, transform Transform) error {
	for _, allowed := range transformsByShape[shape] {
		if transform == allowed {
			return nil
		}
	}
	return fmt.Errorf("transform %q, expected one of %q", transform, transformsByShape[shape])
}

// numberFormatPrecision prints numbers as JavaScript would ("1727280000",
// "0.5"), so a numeric id reads the same as its string form.
const numberFormatPrecision = -1

// readText turns a value into a field string, or "" when it holds none.
func readText(value *v8value.Value, transform Transform) string {
	return transformText(scalarText(value), transform)
}

// ruleText reads a value the way rule says: split, then transform.
func ruleText(value *v8value.Value, rule FieldRule) string {
	return transformText(rule.Split.segment(scalarText(value)), rule.Transform)
}

func transformText(text string, transform Transform) string {
	switch transform {
	case TransformHTMLText:
		return htmltext.ToText(text)
	case TransformTrim:
		return strings.TrimSpace(text)
	}
	return text
}

func scalarText(value *v8value.Value) string {
	if value == nil {
		return ""
	}
	switch value.Kind {
	case v8value.KindString, v8value.KindBigInt:
		return value.Text
	case v8value.KindNumber:
		return strconv.FormatFloat(value.Number, 'f', numberFormatPrecision, 64)
	}
	return ""
}

// readTime turns a value into a time; ok is false when the value is
// missing, zero or not in the transform's shape. A Date is read as such
// whatever the transform, since its unit is fixed.
func readTime(value *v8value.Value, transform Transform) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	if value.Kind == v8value.KindDate {
		return positiveMillis(value.Number)
	}
	switch transform {
	case TransformUnixMS:
		return epochNumber(value, time.Millisecond)
	case TransformUnixS:
		return epochNumber(value, time.Second)
	case TransformISO8601:
		parsed, err := time.Parse(time.RFC3339Nano, value.String())
		return parsed, err == nil
	}
	return time.Time{}, false
}

// epochNumber reads a number, or a numeric string, counted in unit since
// the Unix epoch.
func epochNumber(value *v8value.Value, unit time.Duration) (time.Time, bool) {
	number, err := strconv.ParseFloat(scalarText(value), 64)
	if err != nil {
		return time.Time{}, false
	}
	return positiveMillis(number * float64(unit/time.Millisecond))
}

func positiveMillis(millis float64) (time.Time, bool) {
	if millis <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(millis)), true
}

// readFlag reads a boolean field: only a true boolean counts.
func readFlag(value *v8value.Value) bool {
	return value.IsTrue()
}

// flagTrueText is how a flag reads in a composite text ("true_<chat>_<id>").
const flagTrueText = "true"

// ruleFlag also accepts a split segment reading "true".
func ruleFlag(value *v8value.Value, rule FieldRule) bool {
	return readFlag(value) || rule.Split != nil && rule.Split.segment(scalarText(value)) == flagTrueText
}

// segment returns the Index-th part of text; a nil Split keeps text whole
// and a missing part is "".
func (s *Split) segment(text string) string {
	if s == nil {
		return text
	}
	parts := strings.Split(text, s.Separator)
	if s.Index >= len(parts) {
		return ""
	}
	return parts[s.Index]
}
