package config

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The example and the README tables are the only documentation of the
// fields (JSON has no comments); these tests keep them from drifting.

const exampleConfigPath = "../../config.example.json"

func TestExampleConfigHasOnlyKnownFields(t *testing.T) {
	decoder := json.NewDecoder(bytes.NewReader(readFile(t, exampleConfigPath)))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatalf("config.example.json does not match Config: %v", err)
	}
}

func TestExampleConfigHasEveryField(t *testing.T) {
	example := decodedKeys(t, readFile(t, exampleConfigPath))
	encoded, err := json.Marshal(Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if defaults := decodedKeys(t, encoded); !slices.Equal(example, defaults) {
		t.Fatalf("config.example.json keys %v differ from Config's %v", example, defaults)
	}
}

func TestDocsDescribeEveryField(t *testing.T) {
	encoded, _ := json.Marshal(Defaults())
	for _, doc := range []string{"../../docs/CONFIGURATION.md", "../../README.pt-BR.md"} {
		text := string(readFile(t, doc))
		for _, key := range decodedKeys(t, encoded) {
			if !strings.Contains(text, "`"+key+"`") {
				t.Errorf("%s does not describe config field %q", doc, key)
			}
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// decodedKeys lists the dotted paths of every leaf ("retrieval.top_k");
// arrays are leaves.
func decodedKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	keys := leafKeys(tree, "")
	sort.Strings(keys)
	return keys
}

func leafKeys(tree map[string]any, prefix string) []string {
	var keys []string
	for key, value := range tree {
		nested, isObject := value.(map[string]any)
		if !isObject {
			keys = append(keys, prefix+key)
			continue
		}
		keys = append(keys, leafKeys(nested, prefix+key+".")...)
	}
	return keys
}
