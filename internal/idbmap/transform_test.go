package idbmap

import (
	"testing"
	"time"

	"github.com/chipskein/cade/internal/v8value"
)

func TestReadText(t *testing.T) {
	cases := []struct {
		value     *v8value.Value
		transform Transform
		want      string
	}{
		{str("<p>Olá <b>time</b></p>"), TransformHTMLText, "Olá time"},
		{str("  oi  "), TransformTrim, "oi"},
		{str("  oi  "), TransformNone, "  oi  "},
		{num(1727280000000), TransformNone, "1727280000000"},
		{num(0.5), TransformNone, "0.5"},
		{&v8value.Value{Kind: v8value.KindBigInt, Text: "12345678901234567890"}, TransformNone, "12345678901234567890"},
		{obj(), TransformNone, ""},
		{nil, TransformTrim, ""},
	}
	for _, tc := range cases {
		if got := readText(tc.value, tc.transform); got != tc.want {
			t.Errorf("readText(%+v, %q) = %q, want %q", tc.value, tc.transform, got, tc.want)
		}
	}
}

func TestReadTime(t *testing.T) {
	want := time.UnixMilli(1727280000000)
	cases := []struct {
		name      string
		value     *v8value.Value
		transform Transform
	}{
		{"milliseconds", num(1727280000000), TransformUnixMS},
		{"milliseconds as text", str("1727280000000"), TransformUnixMS},
		{"seconds", num(1727280000), TransformUnixS},
		{"iso8601", str(want.UTC().Format(time.RFC3339Nano)), TransformISO8601},
		{"date with no transform", &v8value.Value{Kind: v8value.KindDate, Number: 1727280000000}, TransformNone},
		{"date under another transform", &v8value.Value{Kind: v8value.KindDate, Number: 1727280000000}, TransformUnixS},
	}
	for _, tc := range cases {
		if got, ok := readTime(tc.value, tc.transform); !ok || !got.Equal(want) {
			t.Errorf("%s: got %v (ok %v), want %v", tc.name, got, ok, want)
		}
	}
}

func TestReadTimeRejectsMissingZeroAndWrongShapes(t *testing.T) {
	cases := map[string]*v8value.Value{"nil": nil, "zero": num(0), "text": str("ontem"), "number without transform": num(1727280000)}
	for name, value := range cases {
		transform := TransformUnixMS
		if name == "number without transform" {
			transform = TransformNone
		}
		if _, ok := readTime(value, transform); ok {
			t.Errorf("%s: expected no time", name)
		}
	}
}

func TestReadFlag(t *testing.T) {
	if !readFlag(&v8value.Value{Kind: v8value.KindBool, Bool: true}) || readFlag(str("true")) || readFlag(nil) {
		t.Fatal("only a true boolean is a true flag")
	}
}

func TestValidateTransformByFieldShape(t *testing.T) {
	if err := validateTransform(shapeTime, TransformHTMLText); err == nil {
		t.Error("html_text on a time field: expected an error")
	}
	if err := validateTransform(shapeText, TransformUnixMS); err == nil {
		t.Error("unix_ms on a text field: expected an error")
	}
	if err := validateTransform(shapeTime, TransformUnixS); err != nil {
		t.Errorf("unix_s on a time field: %v", err)
	}
}
