package tasks

import (
	"time"

	"github.com/chipskein/cade/internal/event"
)

// inheritSpan is how close in time an event without a task link must be to
// a cited event to be attributed to that event's task.
const inheritSpan = 20 * time.Minute

// assignTasks maps each event to a task key: events citing a task (or a PR
// linked to one) directly, others to the nearest cited event within
// inheritSpan — a commit made right after reading the task belongs to it.
// Unassigned events get "".
func assignTasks(events []event.Event, direct func(event.Event) string) []string {
	keys := make([]string, len(events))
	for i, ev := range events {
		keys[i] = direct(ev)
	}
	inherited := make([]string, len(events))
	for i := range events {
		inherited[i] = keys[i]
		if keys[i] == "" {
			inherited[i] = nearestKey(events, keys, i)
		}
	}
	return inherited
}

// nearestKey looks outward from i for the closest directly assigned event.
func nearestKey(events []event.Event, keys []string, i int) string {
	best, bestGap := "", inheritSpan+1
	for j := range events {
		gap := absDuration(events[j].Timestamp.Sub(events[i].Timestamp))
		if keys[j] != "" && gap < bestGap {
			best, bestGap = keys[j], gap
		}
	}
	return best
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
