package devtasks

import (
	"fmt"
	"slices"
	"strings"
)

// PinnedName names a value CI reads for its cache keys, e.g.
// `go tool mage print LLAMA_TAG`.
type PinnedName string

const (
	PinnedLlamaTag            PinnedName = "LLAMA_TAG"
	PinnedLlamaCMakeFlags     PinnedName = "LLAMA_CMAKE_FLAGS"
	PinnedGolangciLintVersion PinnedName = "GOLANGCI_LINT_VERSION"
	PinnedEmbeddingModelURL   PinnedName = "EMBEDDING_MODEL_URL"
	PinnedGenerationModelURL  PinnedName = "GENERATION_MODEL_URL"
)

var pinnedValues = map[PinnedName]string{
	PinnedLlamaTag:            LlamaTag,
	PinnedLlamaCMakeFlags:     strings.Join(llamaCMakeFlags, " "),
	PinnedGolangciLintVersion: GolangciLintVersion,
	PinnedEmbeddingModelURL:   embeddingModelURL,
	PinnedGenerationModelURL:  generationModelURL,
}

// PinnedValue returns the value of name, or an error listing the names.
//
//	tag, err := PinnedValue("LLAMA_TAG") // "b11195"
func PinnedValue(name string) (string, error) {
	value, found := pinnedValues[PinnedName(name)]
	if !found {
		return "", fmt.Errorf("unknown pinned value %q; want one of %s", name, strings.Join(pinnedNames(), ", "))
	}
	return value, nil
}

func pinnedNames() []string {
	names := make([]string, 0, len(pinnedValues))
	for name := range pinnedValues {
		names = append(names, string(name))
	}
	slices.Sort(names)
	return names
}
