package idbdiscovery

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/v8value"
)

// Both grammars allow exactly one JSON shape with fixed spacing, like the
// question planner's, and list paths as literals: the model picks among
// the paths the catalog shows and cannot write any other.

// Time formats the model may name; each maps to an idbmap transform.
const (
	formatUnixMS  = "unix_ms"
	formatUnixS   = "unix_s"
	formatISO8601 = "iso8601"
	formatDate    = "date"
	formatPlain   = "plain"
	formatHTML    = "html_text"
)

var (
	timeFormats = []string{formatUnixMS, formatUnixS, formatISO8601, formatDate}
	textFormats = []string{formatPlain, formatHTML}
)

// Kinds each kind of field can be read from.
var (
	textKinds = []v8value.Kind{v8value.KindString, v8value.KindNumber, v8value.KindBigInt}
	timeKinds = []v8value.Kind{v8value.KindNumber, v8value.KindString, v8value.KindDate}
	flagKinds = []v8value.Kind{v8value.KindBool}
	keepKinds = []v8value.Kind{v8value.KindString}
)

// jsonNull is the GBNF literal of JSON null.
const jsonNull = `"null"`

// maxConditionValues bounds the values a keep condition lists.
const maxConditionValues = 4

// keepValueRule is one condition value: short text with no quotes,
// backslashes or control characters.
const keepValueRule = `keep-value ::= "\"" [^"\\\x00-\x1F]{1,40} "\""`

// storeGrammar lets the model pick one store and, inside it, the path of
// the items that are messages (null when each record is one message).
func storeGrammar(catalog Catalog) string {
	var branches []string
	var rules []string
	for _, store := range catalog.Stores {
		rule := "each-" + store.Label
		branches = append(branches, fmt.Sprintf(`"{\"store\": \"%s\", \"each\": " %s "}"`, store.Label, rule))
		rules = append(rules, alternatives(rule, append([]string{jsonNull}, quotedPaths(eachCandidates(store))...)))
	}
	return strings.Join(append([]string{"root ::= " + strings.Join(branches, " | ")}, rules...), "\n")
}

// eachCandidates are the paths that reach many objects: the values of a
// map, array or set, or of data-like keys (a message map keyed by id).
func eachCandidates(store StoreView) []idbmap.Path {
	var candidates []idbmap.Path
	for _, path := range store.Paths {
		if slices.Contains(path.Kinds, v8value.KindObject) && endsInWildcard(path.Path) {
			candidates = append(candidates, path.Path)
		}
	}
	return candidates
}

func endsInWildcard(path idbmap.Path) bool {
	text := string(path)
	return strings.HasSuffix(text, "[]") || strings.HasSuffix(text, "{}") || strings.HasSuffix(text, "<>") ||
		strings.HasSuffix(text, ">") && strings.Contains(text[strings.LastIndex(text, "."):], "<")
}

// fieldsGrammar lets the model fill the target from the item paths of
// the chosen store; the sender lookup may read any other store.
func fieldsGrammar(catalog Catalog, store StoreView, each idbmap.Path) (string, error) {
	items := store.Under(each)
	sets := map[string][]string{
		"text-path": quotedPaths(pathsOfKinds(items, textKinds)),
		"time-path": quotedPaths(pathsOfKinds(items, timeKinds)),
	}
	for name, literals := range sets {
		if len(literals) == 0 {
			return "", fmt.Errorf("store %s under %q has no %s, expected at least one for the required fields", store.Label, each, name)
		}
	}
	rules := []string{fieldsRoot, requiredList, optionalList("opt-text", "text-path"), timeListRule,
		alternatives("text-path", sets["text-path"]), alternatives("time-path", sets["time-path"]),
		alternatives("time-format", quoted(timeFormats)), alternatives("text-format", quoted(textFormats))}
	rules = append(rules, flagRules(items)...)
	rules = append(rules, keepRules(items)...)
	return strings.Join(append(rules, lookupRules(catalog, store)...), "\n"), nil
}

const fieldsRoot = `root ::= "{\"message_id\": " req-text ", \"conversation_id\": " req-text ", \"sent_at\": " time-list ", \"time_format\": " time-format ", \"sender\": " opt-text ", \"sender_id\": " opt-text ", \"conversation\": " opt-text ", \"text\": " opt-text ", \"text_format\": " text-format ", \"sent_by_me\": " opt-flag ", \"keep\": " keep ", \"sender_lookup\": " lookup "}"`

const (
	requiredList = `req-text ::= "[" text-path (", " text-path)? "]"`
	timeListRule = `time-list ::= "[" time-path (", " time-path)? "]"`
)

func optionalList(rule, item string) string {
	return fmt.Sprintf(`%s ::= "[]" | "[" %s (", " %s)? "]"`, rule, item, item)
}

func flagRules(items []PathView) []string {
	flags := quotedPaths(pathsOfKinds(items, flagKinds))
	if len(flags) == 0 {
		return []string{`opt-flag ::= "[]"`}
	}
	return []string{optionalList("opt-flag", "flag-path"), alternatives("flag-path", flags)}
}

func keepRules(items []PathView) []string {
	paths := quotedPaths(pathsOfKinds(items, keepKinds))
	if len(paths) == 0 {
		return []string{"keep ::= " + jsonNull}
	}
	return []string{
		fmt.Sprintf(`keep ::= `+jsonNull+` | "{\"path\": " keep-path ", \"in\": [" keep-value (", " keep-value){0,%d} "]}"`, maxConditionValues-1),
		alternatives("keep-path", paths), keepValueRule,
	}
}

// lookupRules offer one branch per other store: the key comes from the
// item, match and value from that store's own paths.
func lookupRules(catalog Catalog, chosen StoreView) []string {
	branches := []string{jsonNull}
	var rules []string
	for _, store := range catalog.Stores {
		paths := quotedPaths(pathsOfKinds(store.Paths, textKinds))
		if store.Label == chosen.Label || len(paths) == 0 {
			continue
		}
		rule := "lookup-" + store.Label
		branches = append(branches, rule)
		rules = append(rules,
			fmt.Sprintf(`%s ::= "{\"store\": \"%s\", \"key\": " text-path ", \"match\": " %s-path ", \"value\": " %s-path "}"`, rule, store.Label, rule, rule),
			alternatives(rule+"-path", paths))
	}
	return append([]string{"lookup ::= " + strings.Join(branches, " | ")}, rules...)
}

func pathsOfKinds(paths []PathView, kinds []v8value.Kind) []idbmap.Path {
	var found []idbmap.Path
	for _, path := range paths {
		if slices.ContainsFunc(path.Kinds, func(kind v8value.Kind) bool { return slices.Contains(kinds, kind) }) {
			found = append(found, path.Path)
		}
	}
	return found
}

func alternatives(rule string, literals []string) string {
	return rule + " ::= " + strings.Join(literals, " | ")
}

func quotedPaths(paths []idbmap.Path) []string {
	texts := make([]string, len(paths))
	for i, path := range paths {
		texts[i] = string(path)
	}
	return quoted(texts)
}

// quoted writes each text as a GBNF literal of a JSON string.
func quoted(texts []string) []string {
	escaper := strings.NewReplacer(`\`, `\\\\`, `"`, `\\\"`)
	literals := make([]string, len(texts))
	for i, text := range texts {
		literals[i] = `"\"` + escaper.Replace(text) + `\""`
	}
	return literals
}
