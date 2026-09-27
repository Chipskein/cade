package buildinfo

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// requiredModule matches a require line of go.mod: module path and version.
var requiredModule = regexp.MustCompile(`(?m)^\s*(?:require\s+)?([a-z0-9.-]+\.[a-z]+/\S+)\s+(v\S+)`)

// Every module linked into the binary must be in THIRD_PARTY_NOTICES.md,
// at the version go.mod requires, and so must the llama.cpp tag.
func TestThirdPartyNoticesNameEveryDependency(t *testing.T) {
	notices := readRepoFile(t, "THIRD_PARTY_NOTICES.md")
	for _, match := range requiredModule.FindAllStringSubmatch(readRepoFile(t, "go.mod"), -1) {
		module, version := match[1], match[2]
		if !strings.Contains(notices, module) || !strings.Contains(notices, version) {
			t.Errorf("THIRD_PARTY_NOTICES.md does not name %s %s", module, version)
		}
	}
	tag := regexp.MustCompile(`(?m)^LLAMA_TAG\s*:=\s*(\S+)`).FindStringSubmatch(readRepoFile(t, "Makefile"))
	if tag == nil || !strings.Contains(notices, "`"+tag[1]+"`") {
		t.Errorf("THIRD_PARTY_NOTICES.md does not name the llama.cpp tag %v", tag)
	}
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../../" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
