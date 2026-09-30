package pacing

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/testfakes"
)

var pacingNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// workFor returns work that takes d on clock.
func workFor(clock *testfakes.FakePacingClock, d time.Duration) func() error {
	return func() error {
		clock.Current = clock.Current.Add(d)
		return nil
	}
}

func TestDutyCycleRestsInProportionToWork(t *testing.T) {
	cases := map[int]time.Duration{50: 2 * time.Second, 25: 6 * time.Second, 80: 500 * time.Millisecond}
	for busy, want := range cases {
		clock := &testfakes.FakePacingClock{Current: pacingNow}
		if err := NewDutyCycle(busy, clock).Run(context.Background(), workFor(clock, 2*time.Second)); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(clock.Slept, []time.Duration{want}) {
			t.Fatalf("expected %v of rest after 2s of work at %d%% busy, got %v", want, busy, clock.Slept)
		}
	}
}

func TestDutyCycleAtFullBusyNeverRests(t *testing.T) {
	clock := &testfakes.FakePacingClock{Current: pacingNow}
	if err := NewDutyCycle(fullBusyPercent, clock).Run(context.Background(), workFor(clock, time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(clock.Slept) != 0 {
		t.Fatalf("expected no rest at 100%% busy, got %v", clock.Slept)
	}
}

func TestDutyCycleSkipsRestAfterFailedWork(t *testing.T) {
	clock := &testfakes.FakePacingClock{Current: pacingNow}
	failure := errors.New("model failed")
	err := NewDutyCycle(50, clock).Run(context.Background(), func() error { return failure })
	if !errors.Is(err, failure) || len(clock.Slept) != 0 {
		t.Fatalf("expected the work's error and no rest, got %v after %v", err, clock.Slept)
	}
}

func TestDutyCycleRestEndsWithContext(t *testing.T) {
	clock := &testfakes.FakePacingClock{Current: pacingNow}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewDutyCycle(50, clock).Run(ctx, workFor(clock, time.Second))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation to end the rest, got %v", err)
	}
}

func TestSystemClockSleepEndsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (SystemClock{}).Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a cancelled sleep to return at once, got %v", err)
	}
	if err := (SystemClock{}).Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("expected a short sleep to finish, got %v", err)
	}
}

func TestPacedEmbedderRestsAfterEachEmbedding(t *testing.T) {
	clock := &testfakes.FakePacingClock{Current: pacingNow, Step: time.Second}
	inner := &testfakes.FakeEmbedder{}
	embedder := NewPacedEmbedder(context.Background(), inner, NewDutyCycle(50, clock))
	vector, err := embedder.Embed("abc")
	if err != nil || len(vector) == 0 || len(clock.Slept) != 1 {
		t.Fatalf("expected a vector and one rest, got %v (err %v) after %v", vector, err, clock.Slept)
	}
	if embedder.Close() != nil || !inner.Closed {
		t.Fatal("expected Close to close the inner embedder")
	}
}

func TestPacedDescriberRestsAfterEachImage(t *testing.T) {
	clock := &testfakes.FakePacingClock{Current: pacingNow, Step: time.Second}
	inner := &testfakes.FakeImageDescriber{Reply: "a terminal"}
	describer := NewPacedDescriber(inner, NewDutyCycle(50, clock))
	reply, err := describer.DescribeImage(context.Background(), llm.RGBImage{Width: 1, Height: 1}, "describe", 8)
	if err != nil || reply != "a terminal" || len(clock.Slept) != 1 {
		t.Fatalf("expected the reply and one rest, got %q (err %v) after %v", reply, err, clock.Slept)
	}
	if describer.Close() != nil || !inner.Closed {
		t.Fatal("expected Close to close the inner describer")
	}
}
