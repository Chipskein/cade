package rag

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
)

func TestBuildPromptNumbersEvidenceWithSourceAndDate(t *testing.T) {
	messages := buildPrompt("o que fiz?", []storage.ScoredEvent{scored("a", event.SourceGit, 0.1)}, fixedNow)
	user := messages[1].Content
	if messages[0].Role != llm.RoleSystem || !strings.Contains(user, "[1] Commit git, 2026-09-26 09:00") || !strings.Contains(user, "Pergunta: o que fiz?") {
		t.Fatalf("unexpected prompt:\n%s", user)
	}
}

func TestBuildPromptIncludesCurrentDate(t *testing.T) {
	messages := buildPrompt("q", nil, fixedNow)
	if !strings.Contains(messages[1].Content, "Data e hora atual: 2026-09-26 10:00") {
		t.Fatalf("expected current date in prompt, got %q", messages[1].Content)
	}
}

func TestFormatEvidenceUsesGivenTimeZone(t *testing.T) {
	text := formatEvidence([]storage.ScoredEvent{scored("a", event.SourceGit, 0)}, time.FixedZone("BRT", -3*3600))
	if !strings.Contains(text, "Commit git, 2026-09-26 06:00") {
		t.Fatalf("expected local time 06:00, got %q", text)
	}
}

func TestClipLongText(t *testing.T) {
	if got := clip("  abcdef  ", 3); got != "abc…" {
		t.Fatalf("expected %q, got %q", "abc…", got)
	}
}

func TestIsNotFoundReply(t *testing.T) {
	if !isNotFoundReply("SEM_INFORMACAO") || !isNotFoundReply("   ") || isNotFoundReply("Você fez [1]") {
		t.Fatal("expected marker and blank replies to be not-found, real answers not")
	}
}

func TestCitedIndexesDedupesAndDropsOutOfRange(t *testing.T) {
	got := citedIndexes("veja [2], [1], [2] e [9]", 3)
	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("expected [1 2], got %v", got)
	}
}

func TestSourceLabel(t *testing.T) {
	if sourceLabel(event.SourceTeams) != "Mensagem do Teams" || sourceLabel("slack") != "slack" {
		t.Fatal("expected a label for known sources and the raw name otherwise")
	}
}
