package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/ingest"
)

// FakeClock returns a settable time.
type FakeClock struct {
	Current time.Time
}

func (c *FakeClock) Now() time.Time { return c.Current }

func newTestProgress(interactive bool) (*ingestProgress, *FakeClock, *strings.Builder) {
	clock := &FakeClock{Current: cliNow}
	var out strings.Builder
	env := commandEnv{stderr: &out, toolkit: Toolkit{Now: clock.Now, StderrIsTerminal: interactive}}
	return newIngestProgress(env, "teams x"), clock, &out
}

func TestProgressWaitsForInterval(t *testing.T) {
	progress, clock, out := newTestProgress(true)
	clock.Current = cliNow.Add(100 * time.Millisecond)
	progress.update(ingest.Report{Collected: 1})
	if out.Len() != 0 {
		t.Fatalf("expected nothing before the interval, got %q", out.String())
	}
}

func TestProgressRedrawsInPlaceOnTerminal(t *testing.T) {
	progress, clock, out := newTestProgress(true)
	clock.Current = cliNow.Add(2 * time.Second)
	progress.update(ingest.Report{Collected: 100, Inserted: 60, AlreadyStored: 40})
	expected := clearLine + "teams x: 100 lidos, 60 novos, 40 já existentes · 50/s"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestProgressPrintsLinesWhenNotTerminal(t *testing.T) {
	progress, clock, out := newTestProgress(false)
	clock.Current = cliNow.Add(5 * time.Second)
	progress.update(ingest.Report{Collected: 1})
	clock.Current = cliNow.Add(11 * time.Second)
	progress.update(ingest.Report{Collected: 2})
	if strings.Count(out.String(), "\n") != 1 || strings.Contains(out.String(), "\r") {
		t.Fatalf("expected one plain line after 10s, got %q", out.String())
	}
}

func TestProgressFinishClearsShownLine(t *testing.T) {
	progress, clock, out := newTestProgress(true)
	clock.Current = cliNow.Add(time.Second)
	progress.update(ingest.Report{Collected: 1})
	progress.finish()
	if !strings.HasSuffix(out.String(), clearLine) {
		t.Fatalf("expected the line to be cleared, got %q", out.String())
	}
}

func TestEventsPerSecond(t *testing.T) {
	if eventsPerSecond(10, 0) != 0 || eventsPerSecond(10, 2*time.Second) != 5 {
		t.Fatal("expected 0 for no elapsed time and 5/s for 10 in 2s")
	}
}
