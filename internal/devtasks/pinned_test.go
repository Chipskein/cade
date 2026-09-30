package devtasks

import (
	"strings"
	"testing"
)

func TestPinnedValueReturnsTheLlamaTag(t *testing.T) {
	if got, err := PinnedValue("LLAMA_TAG"); err != nil || got != LlamaTag {
		t.Errorf("PinnedValue = %q, %v", got, err)
	}
}

func TestPinnedValueListsTheKnownNames(t *testing.T) {
	_, err := PinnedValue("NOPE")
	if err == nil || !strings.Contains(err.Error(), `"NOPE"`) || !strings.Contains(err.Error(), "GOLANGCI_LINT_VERSION") {
		t.Errorf("err = %v, want the bad name and the valid ones", err)
	}
}
