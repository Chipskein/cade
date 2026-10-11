package idbschema

import (
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

func object(properties ...v8value.Property) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindObject, Properties: properties}
}

func text(value string) *v8value.Value {
	return &v8value.Value{Kind: v8value.KindString, Text: value}
}

func field(summary StoreSummary, path string) *FieldStat {
	for i := range summary.Fields {
		if summary.Fields[i].Path == path {
			return &summary.Fields[i]
		}
	}
	return nil
}

func chainRecord() webstore.Record {
	message := object(v8value.Property{Key: "content", Value: text("segredo")})
	chain := object(v8value.Property{Key: "messageMap", Value: object(
		v8value.Property{Key: "1727280000000", Value: message},
		v8value.Property{Key: "1727283600000", Value: message},
	)})
	return webstore.Record{Namespace: "Teams:rc:0b0e1f2a-1111-2222-3333-444455556666", Container: "replychains", Value: chain}
}

func TestSummarizeCollapsesIDKeys(t *testing.T) {
	summaries := Summarize([]webstore.Record{chainRecord()})
	stat := field(summaries[0], "$.messageMap.<id>.content")
	if stat == nil || stat.Count != 2 || stat.Kinds[v8value.KindString] != 2 {
		t.Fatalf("expected the id-keyed path counted twice, got %+v", summaries[0].Fields)
	}
}

func TestSummarizeMasksDatabaseName(t *testing.T) {
	summary := Summarize([]webstore.Record{chainRecord()})[0]
	if summary.Database != "Teams:rc:<guid>" || summary.Store != "replychains" {
		t.Fatalf("expected masked database name, got %q / %q", summary.Database, summary.Store)
	}
}

func TestSummarizeNeverIncludesValues(t *testing.T) {
	for _, stat := range Summarize([]webstore.Record{chainRecord()})[0].Fields {
		if strings.Contains(stat.Path, "segredo") {
			t.Fatalf("value leaked into path %q", stat.Path)
		}
	}
}

func TestSummarizeCountsFailuresAndBlobs(t *testing.T) {
	records := []webstore.Record{
		{Namespace: "d", Container: "s", DecodeErr: errors.New("bad tag")},
		{Namespace: "d", Container: "s", DecodeErr: webstore.ErrExternalValue},
	}
	summary := Summarize(records)[0]
	if summary.Records != 2 || summary.Failed != 1 || summary.BlobWrapped != 1 {
		t.Fatalf("unexpected counts %+v", summary)
	}
}

func TestSummarizeKeepsStoreOrder(t *testing.T) {
	records := []webstore.Record{{Namespace: "d", Container: "b", Value: text("x")}, {Namespace: "d", Container: "a", Value: text("y")}}
	summaries := Summarize(records)
	if len(summaries) != 2 || summaries[0].Store != "b" {
		t.Fatalf("expected first-seen order, got %+v", summaries)
	}
}

func TestWalkValueStopsAtCycles(t *testing.T) {
	cyclic := object()
	cyclic.Properties = []v8value.Property{{Key: "self", Value: cyclic}}
	var paths []string
	walkValue("", cyclic, 0, map[*v8value.Value]bool{}, func(path string, _ v8value.Kind) { paths = append(paths, path) })
	if len(paths) != 1 {
		t.Fatalf("expected the cycle to be cut after the root, got %v", paths)
	}
}

func TestWalkChildrenNotation(t *testing.T) {
	value := object(
		v8value.Property{Key: "list", Value: &v8value.Value{Kind: v8value.KindArray, Items: []*v8value.Value{text("a"), nil}}},
		v8value.Property{Key: "tags", Value: &v8value.Value{Kind: v8value.KindSet, Items: []*v8value.Value{text("a")}}},
		v8value.Property{Key: "byName", Value: &v8value.Value{Kind: v8value.KindMap, Entries: []v8value.MapEntry{{Key: text("k"), Value: text("v")}}}},
	)
	seen := map[string]bool{}
	walkValue("", value, 0, map[*v8value.Value]bool{}, func(path string, _ v8value.Kind) { seen[path] = true })
	if !seen["$.list[]"] || !seen["$.tags<>"] || !seen["$.byName{}"] {
		t.Fatalf("unexpected paths %v", seen)
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"content": "content", "imDisplayName": "imDisplayName", "1727280000000": "<id>",
		"8:orgid:abc": "<id>", "19:x@thread.v2": "<id>", "Ana Souza": "<text>",
		"0b0e1f2a-1111-2222-3333-444455556666": "<id>", "deadbeefdeadbeef00": "<id>",
	}
	for key, expected := range cases {
		if got := MaskKey(key); got != expected {
			t.Errorf("MaskKey(%q) = %q, expected %q", key, got, expected)
		}
	}
}

func TestMaskName(t *testing.T) {
	got := MaskName("Teams:conv:ana@corp.com:1234567:v2")
	if got != "Teams:conv:<email>:<n>:v2" {
		t.Fatalf("unexpected masked name %q", got)
	}
}

func TestStablePrefix(t *testing.T) {
	cases := map[string]string{
		"Teams:replychain-manager:react-web-client:0b1c2d3e-0000-1111-2222-333344445555": "Teams:replychain-manager:react-web-client:",
		"Teams:conv:ana@corp.com:v2": "Teams:conv:",
		"model-storage":              "model-storage",
		"cache-1727280000":           "cache-",
	}
	for name, want := range cases {
		if got := StablePrefix(name); got != want {
			t.Errorf("StablePrefix(%q) = %q, want %q", name, got, want)
		}
	}
}
