package cli

import (
	"fmt"
	"time"
)

// etaWindow bounds the moving average to recent items, so a slow start
// (e.g. model loading) does not skew the estimate once the pace picks up.
const etaWindow = 20

// etaTracker estimates time remaining from a moving average of the
// durations of the last etaWindow completed items.
type etaTracker struct {
	now     func() time.Time
	last    time.Time
	samples []time.Duration
}

func newETATracker(now func() time.Time) *etaTracker {
	return &etaTracker{now: now, last: now()}
}

// advance records the duration since the previous advance (or since the
// tracker was created) as one item's cost.
func (t *etaTracker) advance() {
	now := t.now()
	t.samples = append(t.samples, now.Sub(t.last))
	if len(t.samples) > etaWindow {
		t.samples = t.samples[1:]
	}
	t.last = now
}

// remaining estimates the time left for pending items, 0 without samples.
func (t *etaTracker) remaining(pending int) time.Duration {
	if len(t.samples) == 0 || pending <= 0 {
		return 0
	}
	var total time.Duration
	for _, s := range t.samples {
		total += s
	}
	return (total / time.Duration(len(t.samples))) * time.Duration(pending)
}

// formatETA is "~45s" or "~2m30s"; callers skip it entirely for 0.
func formatETA(d time.Duration) string {
	d = d.Round(time.Second)
	minutes := d / time.Minute
	seconds := (d % time.Minute) / time.Second
	if minutes == 0 {
		return fmt.Sprintf("~%ds", seconds)
	}
	return fmt.Sprintf("~%dm%ds", minutes, seconds)
}

// progressBar is "12/50 (24%)"; done may exceed total (final tick), which
// is clamped to 100%.
func progressBar(done, total int) string {
	if total <= 0 {
		return fmt.Sprintf("%d", done)
	}
	percent := min(done*100/total, 100)
	return fmt.Sprintf("%d/%d (%d%%)", done, total, percent)
}

// etaSuffix is " · ETA ~1m40s", or "" when there is nothing left to
// estimate or no samples yet.
func etaSuffix(tracker *etaTracker, pending int) string {
	remaining := tracker.remaining(pending)
	if remaining <= 0 {
		return ""
	}
	return " · ETA " + formatETA(remaining)
}
