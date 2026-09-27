package browsersource

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/chipskein/cade/internal/event"
)

var visitTime = time.Date(2026, 9, 25, 15, 30, 0, 123_000, time.UTC)

func openSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

func buildHistory(t *testing.T, statements ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.sqlite")
	db, _ := openSQLite(path)
	defer db.Close()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	return path
}

func firefoxHistory(t *testing.T) string {
	micros := visitTime.UnixMicro()
	return buildHistory(t,
		`CREATE TABLE moz_places (id INTEGER PRIMARY KEY, url TEXT, title TEXT)`,
		`CREATE TABLE moz_historyvisits (id INTEGER PRIMARY KEY, place_id INTEGER, visit_date INTEGER)`,
		`INSERT INTO moz_places VALUES (1, 'https://go.dev', 'Go'), (2, 'https://x.io', NULL)`,
		`INSERT INTO moz_historyvisits VALUES (1, 1, `+itoa(micros)+`), (2, 1, `+itoa(micros+1)+`), (3, 2, `+itoa(micros+2)+`)`)
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}

func collectAll(t *testing.T, path string) ([]event.Event, error) {
	t.Helper()
	var events []event.Event
	err := NewCollector(openSQLite, path).CollectEvents(context.Background(), func(ev event.Event) error {
		events = append(events, ev)
		return nil
	})
	return events, err
}

func TestCollectFirefoxVisits(t *testing.T) {
	events, err := collectAll(t, firefoxHistory(t))
	if err != nil || len(events) != 3 {
		t.Fatalf("expected 3 visits, got %d (err %v)", len(events), err)
	}
	first := events[0]
	if !first.Timestamp.Equal(visitTime) || first.Content != "Go\nhttps://go.dev" || first.Metadata["browser"] != "firefox" {
		t.Fatalf("unexpected first event %+v", first)
	}
}

func TestRevisitsGetDistinctIDs(t *testing.T) {
	events, _ := collectAll(t, firefoxHistory(t))
	if events[0].UID == events[1].UID {
		t.Fatal("two visits to the same URL must not share a UID")
	}
}

func TestUntitledVisitUsesURLAsContent(t *testing.T) {
	events, _ := collectAll(t, firefoxHistory(t))
	if events[2].Content != "https://x.io" {
		t.Fatalf("expected bare URL content, got %q", events[2].Content)
	}
}

func TestCollectChromiumVisits(t *testing.T) {
	chromeTime := visitTime.UnixMicro() + chromiumEpochOffsetMicros
	path := buildHistory(t,
		`CREATE TABLE urls (id INTEGER PRIMARY KEY, url TEXT, title TEXT)`,
		`CREATE TABLE visits (id INTEGER PRIMARY KEY, url INTEGER, visit_time INTEGER)`,
		`INSERT INTO urls VALUES (7, 'https://sqlite.org', 'SQLite')`,
		`INSERT INTO visits VALUES (1, 7, `+itoa(chromeTime)+`)`)
	events, err := collectAll(t, path)
	if err != nil || len(events) != 1 || !events[0].Timestamp.Equal(visitTime) || events[0].Metadata["browser"] != "chromium" {
		t.Fatalf("expected one chromium visit at %s, got %+v (err %v)", visitTime, events, err)
	}
}

func TestCollectRejectsUnknownSchema(t *testing.T) {
	path := buildHistory(t, `CREATE TABLE something (id INTEGER)`)
	if _, err := collectAll(t, path); err == nil {
		t.Fatal("expected an error for an unrecognised database")
	}
}

func TestCollectLeavesOriginalUntouched(t *testing.T) {
	path := firefoxHistory(t)
	before, _ := os.Stat(path)
	if _, err := collectAll(t, path); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatal("the live history file must not be modified")
	}
}

func TestSnapshotMissingFileFails(t *testing.T) {
	if _, _, err := snapshotHistory(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error for a missing history file")
	}
}

func TestCopyOptionalFileIgnoresMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := copyOptionalFile(filepath.Join(dir, "absent-wal"), filepath.Join(dir, "copy")); err != nil {
		t.Fatalf("expected missing WAL to be ignored, got %v", err)
	}
}

func TestVisitEventStableAcrossPaths(t *testing.T) {
	first := visitEvent("firefox", "/a/places.sqlite", visitTime, "https://go.dev", "Go")
	second := visitEvent("firefox", "/b/places.sqlite", visitTime, "https://go.dev", "Go")
	if first.UID != second.UID {
		t.Fatal("the same visit read from a moved profile must keep its UID")
	}
}
