package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func commitBy(uid, author string, authorship event.Authorship) event.Event {
	return event.Event{UID: uid, Source: event.SourceGit, Timestamp: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), Content: "commit de " + author,
		Metadata: event.Commit{Repository: "/src/api", Hash: uid, Author: author, Authorship: authorship}.Metadata()}
}

// Acceptance (phase 4, see CHANGELOG.md): the timeline shows the user's
// commits; --all-authors shows everyone's.
func TestTimelineHidesOthersCommits(t *testing.T) {
	world := newFakeWorld()
	world.store.Events = []event.Event{commitBy("a", "Eu", event.AuthorshipMine), commitBy("b", "Rui", event.AuthorshipOther), commitBy("c", "Antigo", event.AuthorshipUnknown)}
	_, own, _ := world.run("timeline", "ontem")
	_, all, _ := world.run("timeline", "--all-authors", "ontem")
	if !strings.Contains(own, "commit de Eu") || strings.Contains(own, "commit de Rui") || !strings.Contains(own, "commit de Antigo") || !strings.Contains(all, "commit de Rui") {
		t.Fatalf("expected Rui's commit only with --all-authors, got:\n%s\n---\n%s", own, all)
	}
}

func visitAt(uid string, at time.Time) event.Event {
	return event.Event{UID: uid, Source: event.SourceBrowser, Timestamp: at, Content: "página " + uid,
		Metadata: event.Visit{URL: "https://example.com/" + uid, Title: "página " + uid}.Metadata()}
}

// Phase 27 (#21), step 1: what happened near an event, without a table.
func TestTimelineAroundShowsNeighborsAndMarksTheEvent(t *testing.T) {
	world := newFakeWorld()
	colleague := commitBy("rui", "Rui", event.AuthorshipOther)
	world.store.Events = []event.Event{visitAt("antes", colleague.Timestamp.Add(-15*time.Minute)), colleague,
		commitBy("outro", "Carla", event.AuthorshipOther), visitAt("longe", colleague.Timestamp.Add(5*time.Hour))}
	world.store.Events[2].Timestamp = colleague.Timestamp.Add(time.Minute)
	code, out, stderr := world.run("timeline", "--around", "rui")
	if code != 0 || !strings.Contains(out, "(±2h) — 2 eventos") || !strings.Contains(out, "  09:45  [browser] página antes") ||
		!strings.Contains(out, "▶ 10:00  [git]") || strings.Contains(out, "Carla") || strings.Contains(out, "longe") {
		t.Fatalf("expected the page before and the marked commit only, got code %d:\n%s%s", code, out, stderr)
	}
	if _, wide, _ := world.run("timeline", "--around", "rui", "--window", "6h", "--all-authors"); !strings.Contains(wide, "longe") || !strings.Contains(wide, "Carla") {
		t.Fatalf("expected a 6h window with every author to reach both, got:\n%s", wide)
	}
}

func TestTimelineAroundRejectsUnknownUIDAndDates(t *testing.T) {
	world := newFakeWorld()
	if code, _, stderr := world.run("timeline", "--around", "nada"); code == 0 || !strings.Contains(stderr, `"nada"`) {
		t.Fatalf("expected an error naming the UID, got code %d: %s", code, stderr)
	}
	if code, _, stderr := world.run("timeline", "--around", "nada", "ontem"); code == 0 || !strings.Contains(stderr, "ontem") {
		t.Fatalf("expected --around with a date refused, got code %d: %s", code, stderr)
	}
}

func TestShortDurationDropsZeroUnits(t *testing.T) {
	for duration, want := range map[time.Duration]string{2 * time.Hour: "2h", 90 * time.Minute: "1h30m", 45 * time.Minute: "45m", 30 * time.Second: "30s"} {
		if got := shortDuration(duration); got != want {
			t.Errorf("shortDuration(%s) = %q, want %q", duration, got, want)
		}
	}
}
