package idbdiscovery

import (
	"regexp"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/v8value"
)

// The tests cannot load the model, so assertGrammarWellFormed checks what
// llama.cpp's parser would reject first: a rule used but never defined, a
// rule defined twice, or no root.
var (
	grammarRuleLine = regexp.MustCompile(`^([a-zA-Z0-9-]+) ::= (.+)$`)
	grammarLiteral  = regexp.MustCompile(`"(\\.|[^"\\])*"|\[(\\.|[^\]\\])*\]`)
	grammarRuleName = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9-]*`)
)

func assertGrammarWellFormed(t *testing.T, grammar string) {
	t.Helper()
	defined, used := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(grammar, "\n") {
		match := grammarRuleLine.FindStringSubmatch(line)
		if match == nil || defined[match[1]] {
			t.Fatalf("malformed or duplicate rule line %q", line)
		}
		defined[match[1]] = true
		for _, name := range grammarRuleName.FindAllString(grammarLiteral.ReplaceAllString(match[2], ""), -1) {
			used[name] = true
		}
	}
	for name := range used {
		if !defined[name] {
			t.Fatalf("rule %q is used but not defined in:\n%s", name, grammar)
		}
	}
	if !defined["root"] {
		t.Fatalf("no root rule in:\n%s", grammar)
	}
}

func TestStoreGrammarOffersEachStoreAndItsItemPaths(t *testing.T) {
	grammar := storeGrammar(teamsCatalog(t))
	assertGrammarWellFormed(t, grammar)
	chains := storeNamed(t, teamsCatalog(t), "replychains")
	want := "each-" + chains.Label + ` ::= "null" | "\"$.messageMap.<id>\""`
	if !strings.Contains(grammar, want) {
		t.Fatalf("expected %q in:\n%s", want, grammar)
	}
}

func TestFieldsGrammarRestrictsPathsByKind(t *testing.T) {
	catalog := teamsCatalog(t)
	chains := storeNamed(t, catalog, "replychains")
	grammar, err := fieldsGrammar(catalog, chains, "$.messageMap.<id>")
	if err != nil {
		t.Fatalf("fields grammar: %v", err)
	}
	assertGrammarWellFormed(t, grammar)
	for _, want := range []string{`"\"$.originalArrivalTime\""`, `flag-path ::= "\"$.isSentByCurrentUser\""`, "lookup-"} {
		if !strings.Contains(grammar, want) {
			t.Errorf("expected %q in the grammar", want)
		}
	}
	if timeRule := ruleLine(grammar, "time-path"); strings.Contains(timeRule, "isSentByCurrentUser") || strings.Contains(timeRule, "$.messageMap") {
		t.Errorf("time paths must be item paths of time kinds, got %q", timeRule)
	}
}

func TestFieldsGrammarWithoutFlagsOrConditions(t *testing.T) {
	store := StoreView{Label: "S1", Paths: []PathView{
		{Path: "$.id", Kinds: []v8value.Kind{v8value.KindNumber}},
		{Path: "$.t", Kinds: []v8value.Kind{v8value.KindDate}},
	}}
	grammar, err := fieldsGrammar(Catalog{Stores: []StoreView{store}}, store, "")
	if err != nil {
		t.Fatalf("fields grammar: %v", err)
	}
	assertGrammarWellFormed(t, grammar)
	if ruleLine(grammar, "opt-flag") != `opt-flag ::= "[]"` || ruleLine(grammar, "keep") != `keep ::= "null"` || ruleLine(grammar, "lookup") != `lookup ::= "null"` {
		t.Fatalf("expected empty flag, keep and lookup rules in:\n%s", grammar)
	}
}

func TestFieldsGrammarNeedsTextAndTimePaths(t *testing.T) {
	store := StoreView{Label: "S1", Paths: []PathView{{Path: "$.flag", Kinds: []v8value.Kind{v8value.KindBool}}}}
	if _, err := fieldsGrammar(Catalog{Stores: []StoreView{store}}, store, ""); err == nil || !strings.Contains(err.Error(), "S1") {
		t.Fatalf("expected an error naming the store, got %v", err)
	}
}

func TestQuotedEscapesForGBNFAndJSON(t *testing.T) {
	got := quotedPaths([]idbmap.Path{`$.a"b\c`})[0]
	if got != `"\"$.a\\\"b\\\\c\""` {
		t.Fatalf("unexpected literal %s", got)
	}
}

func TestEachCandidates(t *testing.T) {
	object := []v8value.Kind{v8value.KindObject}
	store := StoreView{Paths: []PathView{
		{Path: "$.messageMap.<id>", Kinds: object}, {Path: "$.items[]", Kinds: object}, {Path: "$.tags[]", Kinds: []v8value.Kind{v8value.KindString}},
		{Path: "$.author", Kinds: object}, {Path: "$.lookup{}", Kinds: object}, {Path: "$.uniq<>", Kinds: object},
	}}
	got := eachCandidates(store)
	if len(got) != 4 || got[0] != "$.messageMap.<id>" || got[3] != "$.uniq<>" {
		t.Fatalf("expected the four object wildcards, got %v", got)
	}
}

func ruleLine(grammar, rule string) string {
	for _, line := range strings.Split(grammar, "\n") {
		if strings.HasPrefix(line, rule+" ::= ") {
			return line
		}
	}
	return ""
}
