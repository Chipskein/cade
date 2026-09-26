package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/timeline"
)

func TestRenderTimelineGroupsByDay(t *testing.T) {
	days, _ := timeline.ParseDayRange("2026-09-24", "2026-09-25", cliNow)
	events := []event.Event{
		{Source: event.SourceFile, Timestamp: time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), Metadata: event.Metadata{"path": "/n.md"}},
		{Source: event.SourceFile, Timestamp: time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC), Metadata: event.Metadata{"path": "/m.md"}},
	}
	var out strings.Builder
	renderTimeline(&out, days, events)
	if strings.Count(out.String(), "── ") != 2 || !strings.Contains(out.String(), "08:00  [file]    /m.md") {
		t.Fatalf("expected two day headers, got:\n%s", out.String())
	}
}

func TestDescribeEventFallsBackToHeadlineForUnknownSource(t *testing.T) {
	ev := event.Event{Source: event.SourceTeams, Content: "\nreunião de sprint\ndetalhes"}
	if got := describeEvent(ev); got != "reunião de sprint" {
		t.Fatalf("expected headline, got %q", got)
	}
}

func TestDescribeTeamsMessage(t *testing.T) {
	ev := event.Event{Source: event.SourceTeams, Content: "Ana: deploy às 10h\nConversa: Release", Metadata: event.Metadata{"conversation": "Release", "conversation_kind": "canal"}}
	if got := describeEvent(ev); got != "Ana: deploy às 10h  (canal Release)" {
		t.Fatalf("unexpected description %q", got)
	}
}

func TestDescribeConversationEmpty(t *testing.T) {
	if got := describeConversation(event.Metadata{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestDescribeVisitWithoutTitle(t *testing.T) {
	if got := describeVisit(event.Metadata{"url": "https://x.io"}); got != "https://x.io" {
		t.Fatalf("expected bare URL, got %q", got)
	}
}

func TestDescribeCommitOriginShortensHash(t *testing.T) {
	got := describeCommitOrigin(event.Metadata{"repository": "/src/app", "hash": "0123456789abcdef"})
	if got != "  (app 01234567)" {
		t.Fatalf("expected short hash, got %q", got)
	}
}

func TestClipLine(t *testing.T) {
	long := strings.Repeat("a", maxDescribedRunes+10)
	if got := []rune(clipLine(long)); len(got) != maxDescribedRunes || got[len(got)-1] != '…' {
		t.Fatalf("expected %d runes ending in ellipsis, got %d", maxDescribedRunes, len(got))
	}
}

func TestRenderAnswerListsAllEvidenceWhenNothingCited(t *testing.T) {
	answer := rag.Answer{Found: true, Text: "resposta", Evidence: []storage.ScoredEvent{{Event: sampleCommit}}}
	var out strings.Builder
	renderAnswer(&out, answer, time.UTC)
	if !strings.Contains(out.String(), "Eventos consultados") || !strings.Contains(out.String(), "[1] [git]") {
		t.Fatalf("expected full evidence list, got:\n%s", out.String())
	}
}

func TestAllEvidenceNumbers(t *testing.T) {
	if got := allEvidenceNumbers(make([]storage.ScoredEvent, 3)); len(got) != 3 || got[2] != 3 {
		t.Fatalf("expected [1 2 3], got %v", got)
	}
}

func TestParseOptionalDaysNilWithoutBounds(t *testing.T) {
	days, err := parseOptionalDays("", "", cliNow)
	if err != nil || days != nil {
		t.Fatalf("expected nil range, got %v (err %v)", days, err)
	}
}

func TestParseOptionalDaysLoneToStartsAtEpoch(t *testing.T) {
	days, err := parseOptionalDays("", "2026-09-20", cliNow)
	if err != nil || days.First.Year() != 1970 || days.Last.Day() != 20 {
		t.Fatalf("expected 1970..2026-09-20, got %v (err %v)", days, err)
	}
}
