package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/export"
	"github.com/chipskein/cade/internal/storage"
)

func exportWorld() *fakeWorld {
	world := newFakeWorld()
	world.store.Events = []event.Event{
		{UID: "old", Source: event.SourceGit, Timestamp: cliNow.AddDate(0, 0, -10), Content: "commit antigo",
			Metadata: event.Commit{Author: "Rui Costa"}.Metadata()},
		{UID: "new", Source: event.SourceGit, Timestamp: cliNow, Content: "commit novo", Metadata: event.Commit{Author: "Rui Costa"}.Metadata()},
	}
	return world
}

func exportedUIDs(t *testing.T, stdout string) []string {
	t.Helper()
	var uids []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var record export.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("line %q is not a record: %v", line, err)
		}
		uids = append(uids, record.UID)
	}
	return uids
}

func TestExportWritesEveryEventAsALineAndCountsOnStderr(t *testing.T) {
	world := exportWorld()
	code, stdout, stderr := world.run("export")
	if uids := exportedUIDs(t, stdout); code != 0 || len(uids) != 2 || uids[0] != "old" || !strings.Contains(stderr, "2 eventos exportados.") {
		t.Fatalf("expected both events and the count, got %d %v %q", code, uids, stderr)
	}
}

// The filters are the ones of `forget --match`.
func TestExportKeepsTheDaysAndSourceAsked(t *testing.T) {
	world := exportWorld()
	day := cliNow.Format(time.DateOnly)
	code, stdout, stderr := world.run("export", "--source", "git", "--from", day, "--to", day)
	if uids := exportedUIDs(t, stdout); code != 0 || len(uids) != 1 || uids[0] != "new" {
		t.Fatalf("expected only today's commit, got %d %v %q", code, uids, stderr)
	}
}

func TestExportRejectsAnUnknownSourceAndABadDate(t *testing.T) {
	for _, args := range [][]string{{"export", "--source", "gti"}, {"export", "--from", "25/09"}} {
		if code, _, stderr := exportWorld().run(args...); code == 0 {
			t.Errorf("%v: expected an error, got success (%q)", args, stderr)
		}
	}
}

func TestSourceAndDaysIncludesTheLastDay(t *testing.T) {
	filter, err := sourceAndDays("git", "2026-09-01", "2026-09-02")
	want := storage.EventFilter{Source: event.SourceGit, From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}
	if err != nil || filter.Source != want.Source || !filter.From.Equal(want.From) || !filter.To.Equal(want.To) {
		t.Fatalf("expected %+v, got %+v (err %v)", want, filter, err)
	}
}
