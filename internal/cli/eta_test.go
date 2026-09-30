package cli

import (
	"testing"
	"time"
)

func TestETATrackerNeedsASampleBeforeEstimating(t *testing.T) {
	clock := &FakeClock{Current: cliNow}
	tracker := newETATracker(clock.Now)
	if tracker.remaining(10) != 0 {
		t.Fatalf("expected 0 without samples, got %v", tracker.remaining(10))
	}
}

func TestETATrackerAveragesRecentSamples(t *testing.T) {
	clock := &FakeClock{Current: cliNow}
	tracker := newETATracker(clock.Now)
	clock.Current = clock.Current.Add(2 * time.Second)
	tracker.advance()
	clock.Current = clock.Current.Add(4 * time.Second)
	tracker.advance()
	if got := tracker.remaining(3); got != 9*time.Second {
		t.Fatalf("expected 3s average * 3 pending = 9s, got %v", got)
	}
}

func TestETATrackerKeepsOnlyTheRecentWindow(t *testing.T) {
	clock := &FakeClock{Current: cliNow}
	tracker := newETATracker(clock.Now)
	clock.Current = clock.Current.Add(time.Hour)
	tracker.advance()
	for i := 0; i < etaWindow; i++ {
		clock.Current = clock.Current.Add(time.Second)
		tracker.advance()
	}
	if got := tracker.remaining(1); got != time.Second {
		t.Fatalf("expected the hour-long first sample dropped from the window, got %v", got)
	}
}

func TestFormatETA(t *testing.T) {
	cases := map[time.Duration]string{45 * time.Second: "~45s", 150 * time.Second: "~2m30s"}
	for d, want := range cases {
		if got := formatETA(d); got != want {
			t.Fatalf("formatETA(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestProgressBar(t *testing.T) {
	if got := progressBar(12, 50); got != "12/50 (24%)" {
		t.Fatalf("expected a done/total percentage, got %q", got)
	}
	if got := progressBar(51, 50); got != "51/50 (100%)" {
		t.Fatalf("expected percentage clamped at 100%%, got %q", got)
	}
	if got := progressBar(3, 0); got != "3" {
		t.Fatalf("expected just the count without a known total, got %q", got)
	}
}

func TestETASuffixIsEmptyWithNothingLeft(t *testing.T) {
	clock := &FakeClock{Current: cliNow}
	tracker := newETATracker(clock.Now)
	clock.Current = clock.Current.Add(time.Second)
	tracker.advance()
	if got := etaSuffix(tracker, 0); got != "" {
		t.Fatalf("expected no ETA with nothing pending, got %q", got)
	}
	if got := etaSuffix(tracker, 5); got != " · ETA ~5s" {
		t.Fatalf("expected an ETA suffix, got %q", got)
	}
}
