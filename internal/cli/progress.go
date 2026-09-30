package cli

import (
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingestrun"
)

// On a terminal the progress line is redrawn often; when stderr goes to a
// file or pipe, a line every few seconds keeps logs readable.
const (
	interactiveProgressInterval = 200 * time.Millisecond
	loggedProgressInterval      = 10 * time.Second
)

// ingestProgress prints the running totals of one ingestion job, and
// records them for `ingest status`.
type ingestProgress struct {
	status   statusLine
	label    string
	language Language
	now      func() time.Time
	started  time.Time
	// total is the collector's estimate of its events; 0 when unknown.
	total       int
	tracker     *etaTracker
	run         *ingestRun
	lastPrinted time.Time
}

func newIngestProgress(env commandEnv, label string, total int) *ingestProgress {
	started := env.toolkit.Now()
	return &ingestProgress{
		status: statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal},
		label:  label, language: env.language, now: env.toolkit.Now, started: started, lastPrinted: started,
		total: total, tracker: newETATracker(env.toolkit.Now, eventETAWindow), run: env.ingestRun,
	}
}

// update prints the totals if the refresh interval has elapsed.
func (p *ingestProgress) update(report ingest.Report) {
	p.tracker.advance()
	now := p.now()
	p.run.update(ingestrun.StageIngesting, func() string { return p.line(report, now) })
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

// line has an ETA only when the collector estimated its events (files,
// commits): the others stream an unknown total, where elapsed time plus
// rate is the closest honest signal of how long the run has been going.
func (p *ingestProgress) line(report ingest.Report, now time.Time) string {
	elapsed := now.Sub(p.started)
	return fmt.Sprintf("%s: %s, %s · %.0f/s · %s%s", p.label, p.collected(report), ingestTally(report, p.language),
		eventsPerSecond(report.Collected, elapsed), formatElapsed(elapsed), etaSuffix(p.tracker, p.total-report.Collected))
}

// collected is "120 lidos", or "120 lidos de 500 (24%)" with an estimate.
func (p *ingestProgress) collected(report ingest.Report) string {
	read := p.language.count(report.Collected, collectedNoun)
	if p.total <= 0 {
		return read
	}
	percent := min(report.Collected*100/p.total, 100)
	return fmt.Sprintf(p.language.pick("%s de %d (%d%%)", "%s of %d (%d%%)"), read, p.total, percent)
}

// formatElapsed is "0:45", "12:03" or "1:02:03"; a running clock, not an ETA.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	seconds := (d % time.Minute) / time.Second
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func eventsPerSecond(count int, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(count) / elapsed.Seconds()
}
