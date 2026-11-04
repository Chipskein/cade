package chunking

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestShortTextIsOneChunk(t *testing.T) {
	if spans := Split("commit curto"); len(spans) != 1 || spans[0] != (Span{0, 12}) {
		t.Fatalf("expected one span, got %v", spans)
	}
	if spans := Split(""); len(spans) != 1 || spans[0] != (Span{0, 0}) {
		t.Fatalf("expected one empty span, got %v", spans)
	}
}

func longNote(sections int) string {
	var builder strings.Builder
	for i := range sections {
		builder.WriteString("## Seção " + string(rune('A'+i)) + "\n")
		builder.WriteString(strings.Repeat("Discutimos o volume atual e os riscos de indisponibilidade. ", 12) + "\n\n")
	}
	return builder.String()
}

// Every chunk fits, the chunks cover the whole text in order, and
// consecutive chunks overlap by at most Overlap.
func TestSplitCoversTextWithinLimits(t *testing.T) {
	text := longNote(20)
	spans := Split(text)
	if len(spans) < 10 || spans[0].Start != 0 || spans[len(spans)-1].End != len(text) {
		t.Fatalf("expected many spans covering the text, got %d", len(spans))
	}
	for i, span := range spans {
		if span.End-span.Start > MaxChars || span.End <= span.Start {
			t.Fatalf("span %d %v exceeds %d or is empty", i, span, MaxChars)
		}
		if i > 0 && (span.Start > spans[i-1].End || spans[i-1].End-span.Start > Overlap) {
			t.Fatalf("span %d %v leaves a gap or overlaps more than %d after %v", i, span, Overlap, spans[i-1])
		}
	}
}

func TestSplitPrefersHeadings(t *testing.T) {
	text := longNote(6)
	for _, span := range Split(text)[1:] {
		chunk := text[span.Start:span.End]
		if !strings.Contains(chunk, "## Seção") {
			t.Fatalf("expected each chunk to carry a heading, got %q", chunk[:40])
		}
	}
}

// Text without separators is cut at the limit, never inside a rune.
func TestSplitKeepsRunesWhole(t *testing.T) {
	text := strings.Repeat("ç", 2000)
	for _, span := range Split(text) {
		if !utf8.ValidString(text[span.Start:span.End]) {
			t.Fatalf("span %v cuts a rune", span)
		}
	}
}

// The answer in the second half of a long note ends up in some chunk.
func TestAnswerDeepInNoteIsInOneChunk(t *testing.T) {
	answer := "O timeout do gateway de pagamentos foi aumentado para 45 segundos."
	text := longNote(15) + answer + "\n" + longNote(5)
	found := false
	for _, span := range Split(text) {
		found = found || strings.Contains(text[span.Start:span.End], answer)
	}
	if !found {
		t.Fatal("expected the answer whole inside one chunk")
	}
}

func TestSplitKeepsEachChunkOnOnePage(t *testing.T) {
	long := strings.Repeat("palavra ", 400)
	text := "relatorio.pdf\nprimeira\f\f" + long + "\fúltima"
	spans := Split(text)
	if len(spans) < 4 {
		t.Fatalf("got %d spans, want the long page in several", len(spans))
	}
	for _, span := range spans {
		if strings.Contains(text[span.Start:span.End], PageSeparator) {
			t.Errorf("span %+v holds a page separator", span)
		}
		if strings.TrimSpace(text[span.Start:span.End]) == "" {
			t.Errorf("span %+v is blank", span)
		}
	}
	if got := PageOf(text, spans[0].Start); got != 1 {
		t.Errorf("first chunk on page %d, want 1", got)
	}
	if got := PageOf(text, spans[1].Start); got != 3 {
		t.Errorf("second chunk on page %d, want 3, past the blank page", got)
	}
	last := spans[len(spans)-1]
	if text[last.Start:last.End] != "última" || PageOf(text, last.Start) != 4 {
		t.Errorf("last chunk = %q on page %d", text[last.Start:last.End], PageOf(text, last.Start))
	}
}

func TestSplitOfShortPagesDoesNotMergeThem(t *testing.T) {
	if spans := Split("um\fdois"); len(spans) != 2 {
		t.Errorf("got %d spans, want one per page", len(spans))
	}
}

func TestSplitOfOnlyBlankPagesStillHasASpan(t *testing.T) {
	text := " \f\n\f"
	spans := Split(text)
	if len(spans) != 1 || spans[0] != (Span{Start: 0, End: len(text)}) {
		t.Errorf("got %+v, want the whole text, as for any other short text", spans)
	}
}

func TestPageOfClampsTheOffset(t *testing.T) {
	cases := map[int]int{-5: 1, 0: 1, 1: 1, 2: 2, 3: 2, 99: 3}
	for offset, want := range cases {
		if got := PageOf("a\fb\f", offset); got != want {
			t.Errorf("PageOf(%d) = %d, want %d", offset, got, want)
		}
	}
}
