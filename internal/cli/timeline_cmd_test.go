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
