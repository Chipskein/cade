package cli

import (
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/ingest"
)

// On a terminal the progress line is redrawn often; when stderr goes to a
// file or pipe, a line every few seconds keeps logs readable.
const (
	interactiveProgressInterval = 200 * time.Millisecond
	loggedProgressInterval      = 10 * time.Second
)

// ingestProgress prints the running totals of one ingestion job.
type ingestProgress struct {
	status      statusLine
	label       string
	language    Language
	now         func() time.Time
	started     time.Time
	lastPrinted time.Time
}

func newIngestProgress(env commandEnv, label string) *ingestProgress {
	started := env.toolkit.Now()
	return &ingestProgress{
		status: statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal},
		label:  label, language: env.language, now: env.toolkit.Now, started: started, lastPrinted: started,
	}
}

// update prints the totals if the refresh interval has elapsed.
func (p *ingestProgress) update(report ingest.Report) {
	now := p.now()
	if now.Sub(p.lastPrinted) < p.interval() {
		return
	}
	p.lastPrinted = now
	p.status.show(p.line(report, now))
}

// finish erases the in-place line so the final summary starts clean.
func (p *ingestProgress) finish() {
	p.status.clear()
}

func (p *ingestProgress) interval() time.Duration {
	return progressInterval(p.status.interactive)
}

// progressInterval is how often a status line refreshes: often on a
// terminal, and a few seconds apart when logged to a file or pipe.
func progressInterval(interactive bool) time.Duration {
	if interactive {
		return interactiveProgressInterval
	}
	return loggedProgressInterval
}

// line has no ETA: directory/text collection streams from an unbounded
// walk, so the total is unknown until it finishes. Elapsed time plus rate
// is the closest honest signal of how long the run has been going.
func (p *ingestProgress) line(report ingest.Report, now time.Time) string {
	elapsed := now.Sub(p.started)
	return fmt.Sprintf("%s: %s, %s · %.0f/s · %s", p.label, p.language.count(report.Collected, collectedNoun), ingestTally(report, p.language),
		eventsPerSecond(report.Collected, elapsed), formatElapsed(elapsed))
}

// formatElapsed is "0:45" or "12:03"; a plain running clock, not an ETA.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	minutes := d / time.Minute
	seconds := (d % time.Minute) / time.Second
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func eventsPerSecond(count int, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(count) / elapsed.Seconds()
}
