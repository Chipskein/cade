package devtasks

import (
	"errors"
	"fmt"
	"strings"
)

// formattedDirs are the Go sources gofmt checks.
var formattedDirs = []string{"cmd", "internal", "magefiles"}

// Build compiles the default binary into bin/cade: NVIDIA since 7cfdba1.
func (t *Tasks) Build() error {
	return t.BuildCUDA()
}

// Build compiles the CPU binary into bin/cade.
func (t *Tasks) BuildCPU() error {
	return t.buildBinary(LlamaCPU, fts5Tag)
}

// BuildCUDA compiles the CUDA binary into bin/cade.
func (t *Tasks) BuildCUDA() error {
	return t.buildBinary(LlamaCUDA, fts5Tag, cudaTag)
}

func (t *Tasks) buildBinary(variant LlamaVariant, tags ...string) error {
	if err := t.EnsureLlama(variant); err != nil {
		return err
	}
	stamp := t.resolveBuildStamp()
	return t.runner.Run(Command{Name: "go", Args: []string{
		"build", "-tags", joinTags(tags...), "-ldflags", stamp.linkerFlags(), "-o", binaryPath, mainPackage,
	}})
}

// Test runs every unit test.
func (t *Tasks) Test() error {
	if err := t.EnsureLlama(LlamaCPU); err != nil {
		return err
	}
	return t.runner.Run(Command{Name: "go", Args: []string{"test", "-tags", fts5Tag, allPackages}})
}

// Cover runs the tests with a coverage profile and prints the per-function
// table, with the total last.
func (t *Tasks) Cover() error {
	if err := t.EnsureLlama(LlamaCPU); err != nil {
		return err
	}
	test := Command{Name: "go", Args: []string{"test", "-tags", fts5Tag, "-coverprofile=" + coverageProfile, allPackages}}
	if err := t.runner.Run(test); err != nil {
		return err
	}
	return t.runner.Run(Command{Name: "go", Args: []string{"tool", "cover", "-func=" + coverageProfile}})
}

// Vet runs go vet.
func (t *Tasks) Vet() error {
	if err := t.EnsureLlama(LlamaCPU); err != nil {
		return err
	}
	return t.runner.Run(Command{Name: "go", Args: []string{"vet", "-tags", fts5Tag, allPackages}})
}

// Lint runs golangci-lint (linters in .golangci.yml). Install the pinned
// version with: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@<GolangciLintVersion>
func (t *Tasks) Lint() error {
	if err := t.EnsureLlama(LlamaCPU); err != nil {
		return err
	}
	return t.runner.Run(Command{Name: "golangci-lint", Args: []string{"run", allPackages}})
}

// Format rewrites the sources with gofmt.
func (t *Tasks) Format() error {
	return t.runner.Run(Command{Name: "gofmt", Args: append([]string{"-w"}, formattedDirs...)})
}

// FormatCheck fails listing the files gofmt would change.
func (t *Tasks) FormatCheck() error {
	unformatted, err := t.runner.Output(Command{Name: "gofmt", Args: append([]string{"-l"}, formattedDirs...)})
	if err != nil {
		return err
	}
	if unformatted != "" {
		return errors.New("gofmt would change:\n" + unformatted)
	}
	return nil
}

// buildStamp is what `cade version` reports: the tag (or commit) and the
// commit's date, so two builds of one commit report the same thing.
type buildStamp struct {
	version, commit, date string
}

const unknownVersion = "dev"

func (t *Tasks) resolveBuildStamp() buildStamp {
	return buildStamp{
		version: t.stampPart(EnvVersion, unknownVersion, "describe", "--tags", "--always", "--dirty"),
		commit:  t.stampPart(EnvCommit, "", "rev-parse", "--short=12", "HEAD"),
		date:    t.stampPart(EnvBuildDate, "", "log", "-1", "--format=%cd", "--date=format:%Y-%m-%d"),
	}
}

// stampPart prefers the variable, then git, then fallback (a tarball
// without .git).
func (t *Tasks) stampPart(name EnvVar, fallback string, gitArgs ...string) string {
	if value, found := t.env.LookupEnv(name); found {
		return value
	}
	value, err := t.runner.Output(Command{Name: "git", Args: gitArgs})
	if err != nil || value == "" {
		return fallback
	}
	return value
}

func (s buildStamp) linkerFlags() string {
	variables := [][2]string{{"version", s.version}, {"commit", s.commit}, {"date", s.date}, {"llamaTag", LlamaTag}}
	flags := make([]string, 0, len(variables))
	for _, variable := range variables {
		flags = append(flags, fmt.Sprintf("-X %s.%s=%s", buildInfoPackage, variable[0], variable[1]))
	}
	return strings.Join(flags, " ")
}
