package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
)

func TestStatusLineInPlaceOnTerminal(t *testing.T) {
	var out strings.Builder
	status := statusLine{out: &out, interactive: true}
	status.show("a")
	status.show("b")
	status.clear()
	status.clear()
	if out.String() != clearLine+"a"+clearLine+"b"+clearLine {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestStatusLinePlainWhenNotTerminal(t *testing.T) {
	var out strings.Builder
	status := statusLine{out: &out}
	status.show("a")
	status.clear()
	if out.String() != "a\n" {
		t.Fatalf("expected a plain line, got %q", out.String())
	}
}

func streamPieces(pieces ...string) (*answerStream, string) {
	var out strings.Builder
	stream := &answerStream{out: &out}
	for _, piece := range pieces {
		stream.write(piece)
	}
	return stream, out.String()
}

func TestAnswerStreamPassesNormalText(t *testing.T) {
	stream, out := streamPieces("  Você ", "fez ", "[1].")
	if out != "Você fez [1]." || !stream.wroteAny() {
		t.Fatalf("expected streamed text, got %q", out)
	}
}

func TestAnswerStreamSuppressesMarker(t *testing.T) {
	stream, out := streamPieces("SEM", "_INFOR", "MACAO")
	if out != "" || stream.wroteAny() {
		t.Fatalf("the not-found marker must not be shown, got %q", out)
	}
}

func TestAnswerStreamReleasesTextThatOnlyStartedLikeMarker(t *testing.T) {
	_, out := streamPieces("SE", "MPRE que possível")
	if out != "SEMPRE que possível" {
		t.Fatalf("expected the held text once it diverged, got %q", out)
	}
}

func newTestSession(interactive bool) (*askSession, *strings.Builder, *strings.Builder) {
	var stdout, stderr strings.Builder
	env := commandEnv{stdout: &stdout, stderr: &stderr, toolkit: Toolkit{StderrIsTerminal: interactive}}
	return newAskSession(env), &stdout, &stderr
}

func TestAskSessionShowsStagesAndPromptProgress(t *testing.T) {
	session, _, stderr := newTestSession(true)
	observer := session.observer()
	observer.StageStarted(rag.StageSearching)
	observer.Generation.PromptProcessed(50, 200)
	if !strings.Contains(stderr.String(), "Buscando eventos…") || !strings.Contains(stderr.String(), "Lendo contexto: 25%") {
		t.Fatalf("unexpected status output %q", stderr.String())
	}
}

func TestAskSessionSkipsPercentagesWhenNotTerminal(t *testing.T) {
	session, _, stderr := newTestSession(false)
	session.observer().Generation.PromptProcessed(50, 200)
	if stderr.Len() != 0 {
		t.Fatalf("expected no percentage lines in logs, got %q", stderr.String())
	}
}

func TestAskSessionRendersSourcesAfterStream(t *testing.T) {
	session, stdout, _ := newTestSession(true)
	session.observer().Generation.TokenGenerated("Você fez [1].")
	answer := rag.Answer{Found: true, Text: "Você fez [1].", Cited: []int{1}, Evidence: []storage.ScoredEvent{{Event: sampleCommit}}}
	session.render(answer, time.UTC)
	if strings.Count(stdout.String(), "Você fez [1].") != 1 || !strings.Contains(stdout.String(), "\n\nFontes citadas:") {
		t.Fatalf("expected the text once followed by sources, got %q", stdout.String())
	}
}

func TestAskSessionRendersNotFoundWhenNothingStreamed(t *testing.T) {
	session, stdout, _ := newTestSession(true)
	session.render(rag.Answer{}, time.UTC)
	if !strings.Contains(stdout.String(), "Não encontrei informação") {
		t.Fatalf("expected not-found message, got %q", stdout.String())
	}
}
