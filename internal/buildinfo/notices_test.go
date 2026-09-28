package buildinfo

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/devtasks"
)

// requiredModule matches a require line of go.mod: module path and version.
var requiredModule = regexp.MustCompile(`(?m)^\s*(?:require\s+)?([a-z0-9.-]+\.[a-z]+/\S+)\s+(v\S+)`)

// toolDirective matches a tool line of go.mod: a development tool (Mage),
// built by `go tool` and never linked into cade.
var toolDirective = regexp.MustCompile(`(?m)^\s*(?:tool\s+)?([a-z0-9.-]+\.[a-z]+/\S+)\s*$`)

// Every module linked into the binary must be in THIRD_PARTY_NOTICES.md,
// at the version go.mod requires, and so must the llama.cpp tag.
func TestThirdPartyNoticesNameEveryDependency(t *testing.T) {
	notices := readRepoFile(t, "THIRD_PARTY_NOTICES.md")
	goMod := readRepoFile(t, "go.mod")
	tools := toolModules(goMod)
	for _, match := range requiredModule.FindAllStringSubmatch(goMod, -1) {
		module, version := match[1], match[2]
		if tools[module] {
			continue
		}
		if !strings.Contains(notices, module) || !strings.Contains(notices, version) {
			t.Errorf("THIRD_PARTY_NOTICES.md does not name %s %s", module, version)
		}
	}
	if !strings.Contains(notices, "`"+devtasks.LlamaTag+"`") {
		t.Errorf("THIRD_PARTY_NOTICES.md does not name the llama.cpp tag %s", devtasks.LlamaTag)
	}
}

// projectLicense is cade's license since #10: the LICENSE header that
// names its version and the SPDX identifier the notices declare.
var projectLicense = struct{ header, spdx string }{
	header: "GNU GENERAL PUBLIC LICENSE\n                       Version 3, 29 June 2007",
	spdx:   "`GPL-3.0-or-later`",
}

// The LICENSE text, the notices and the licensing doc must name the same
// license, so a later change cannot update one and forget the others.
func TestLicenseMatchesDeclaredLicense(t *testing.T) {
	if license := readRepoFile(t, "LICENSE"); !strings.Contains(license, projectLicense.header) {
		t.Errorf("LICENSE does not start with %q", projectLicense.header)
	}
	for _, name := range []string{"THIRD_PARTY_NOTICES.md", "docs/LICENSING.md", "docs/LICENSING.pt-BR.md"} {
		if !strings.Contains(readRepoFile(t, name), projectLicense.spdx) {
			t.Errorf("%s does not declare %s", name, projectLicense.spdx)
		}
	}
}

// toolModules are the modules go.mod declares as tools; a tool line names a
// package, which is the module itself for Mage.
func toolModules(goMod string) map[string]bool {
	tools := map[string]bool{}
	for _, block := range regexp.MustCompile(`(?ms)^tool\s*\((.*?)\)|^tool\s+\S+$`).FindAllString(goMod, -1) {
		for _, match := range toolDirective.FindAllStringSubmatch(block, -1) {
			tools[match[1]] = true
		}
	}
	return tools
}

func TestToolModulesReadsSingleAndBlockTools(t *testing.T) {
	goMod := "require example.com/lib v1.0.0\n\ntool example.com/one\n\ntool (\n\texample.com/two\n)\n"
	tools := toolModules(goMod)
	if len(tools) != 2 || !tools["example.com/one"] || !tools["example.com/two"] {
		t.Errorf("toolModules = %v, want example.com/one and example.com/two", tools)
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
