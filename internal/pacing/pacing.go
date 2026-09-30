// Package pacing keeps the models of a long ingestion busy for at most a
// share of the wall clock, so the machine stays usable while it runs
// (issue #41). llama.cpp has no utilization cap and a GPU cannot be
// niced: resting between calls is what lowers the average load.
package pacing

import (
	"context"
	"time"
)

// fullBusyPercent is working all the time, which never rests.
const fullBusyPercent = 100

// Clock is the time source pacing measures and sleeps on.
type Clock interface {
	Now() time.Time
	// Sleep waits d, returning early with ctx's error when it ends.
	Sleep(ctx context.Context, d time.Duration) error
}

// SystemClock is the wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

func (SystemClock) Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// DutyCycle rests after each piece of work so that work takes busyPercent
// of the time: at 50, a 2 s image description is followed by 2 s of rest.
type DutyCycle struct {
	busyPercent int
	clock       Clock
}

// NewDutyCycle paces to busyPercent (1–100, validated by the config).
//
//	cycle := pacing.NewDutyCycle(50, pacing.SystemClock{})
func NewDutyCycle(busyPercent int, clock Clock) *DutyCycle {
	return &DutyCycle{busyPercent: busyPercent, clock: clock}
}

// Run does work, then rests in proportion to how long it took. A failed
// work does not rest: the run is about to stop.
func (d *DutyCycle) Run(ctx context.Context, work func() error) error {
	started := d.clock.Now()
	if err := work(); err != nil {
		return err
	}
	rest := d.restAfter(d.clock.Now().Sub(started))
	if rest <= 0 {
		return nil
	}
	return d.clock.Sleep(ctx, rest)
}

func (d *DutyCycle) restAfter(busy time.Duration) time.Duration {
	if d.busyPercent <= 0 || d.busyPercent >= fullBusyPercent {
		return 0
	}
	return busy * time.Duration(fullBusyPercent-d.busyPercent) / time.Duration(d.busyPercent)
}
