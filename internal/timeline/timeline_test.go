package timeline

import (
	"context"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testfakes"
)

var saoPaulo = time.FixedZone("BRT", -3*3600)
var now = time.Date(2026, 9, 26, 1, 30, 0, 0, saoPaulo)

func TestParseDayRangeSingleDay(t *testing.T) {
	days, err := ParseDayRange("2026-09-25", "", now)
	if err != nil || !days.IsSingleDay() || days.String() != "2026-09-25" {
		t.Fatalf("expected single day 2026-09-25, got %v (err %v)", days, err)
	}
}

func TestParseDayRangeUsesLocalMidnight(t *testing.T) {
	days, _ := ParseDayRange("2026-09-25", "", now)
	if !days.Start().Equal(time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected start at 03:00 UTC (BRT midnight), got %s", days.Start().UTC())
	}
}

func TestParseDayRangeEndIsExclusiveNextMidnight(t *testing.T) {
	days, _ := ParseDayRange("2026-09-01", "2026-09-07", now)
	if !days.End().Equal(time.Date(2026, 9, 8, 0, 0, 0, 0, saoPaulo)) || days.String() != "2026-09-01 a 2026-09-07" {
		t.Fatalf("unexpected range %v ending %s", days, days.End())
	}
}

func TestParseDayRangeRejectsReversedRange(t *testing.T) {
	if _, err := ParseDayRange("2026-09-07", "2026-09-01", now); err == nil {
		t.Fatal("expected an error for end before start")
	}
}

func TestParseDayRangeRejectsBadEnd(t *testing.T) {
	if _, err := ParseDayRange("2026-09-07", "amanhã", now); err == nil {
		t.Fatal("expected an error for an unparseable end")
	}
}

func TestParseDayRelativeWords(t *testing.T) {
	today, _ := ParseDay("hoje", now)
	yesterday, _ := ParseDay("Ontem", now)
	if today.Day() != 26 || yesterday.Day() != 25 || today.Hour() != 0 {
		t.Fatalf("expected 26 and 25 at midnight, got %s and %s", today, yesterday)
	}
}

func TestParseDayRejectsGarbage(t *testing.T) {
	if _, err := ParseDay("25/09/2026", now); err == nil {
		t.Fatal("expected an error for a non-ISO date")
	}
}

func TestMidnight(t *testing.T) {
	if got := midnight(now); got.Hour() != 0 || got.Day() != 26 || got.Location() != saoPaulo {
		t.Fatalf("expected local midnight of the 26th, got %s", got)
	}
}

func timelineStore() *testfakes.FakeEventStore {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{
		{UID: "late", Source: event.SourceBrowser, Timestamp: time.Date(2026, 9, 25, 20, 0, 0, 0, saoPaulo)},
		{UID: "early", Source: event.SourceGit, Timestamp: time.Date(2026, 9, 25, 9, 0, 0, 0, saoPaulo)},
		{UID: "other-day", Source: event.SourceGit, Timestamp: time.Date(2026, 9, 26, 0, 0, 0, 0, saoPaulo)},
	}
	return store
}

func TestListReturnsDayAcrossSourcesInOrder(t *testing.T) {
	days, _ := ParseDayRange("2026-09-25", "", now)
	events, err := NewLister(timelineStore()).List(context.Background(), days, "")
	if err != nil || len(events) != 2 || events[0].UID != "early" || events[1].UID != "late" {
		t.Fatalf("expected [early late], got %+v (err %v)", events, err)
	}
}

func TestListFiltersBySource(t *testing.T) {
	days, _ := ParseDayRange("2026-09-25", "", now)
	events, _ := NewLister(timelineStore()).List(context.Background(), days, event.SourceBrowser)
	if len(events) != 1 || events[0].UID != "late" {
		t.Fatalf("expected only the browser event, got %+v", events)
	}
}

func TestFilterBySourceEmptyKeepsAll(t *testing.T) {
	events := []event.Event{{UID: "a"}, {UID: "b"}}
	if got := filterBySource(events, ""); len(got) != 2 {
		t.Fatalf("expected both events, got %d", len(got))
	}
}
