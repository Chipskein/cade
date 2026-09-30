package testfakes

import (
	"context"
	"time"
)

// FakePacingClock is a settable clock whose Sleep records the duration and
// moves the time forward instead of waiting.
type FakePacingClock struct {
	Current time.Time
	// Step moves the time forward on every Now, so work measured between
	// two readings takes Step.
	Step  time.Duration
	Slept []time.Duration
}

func (c *FakePacingClock) Now() time.Time {
	c.Current = c.Current.Add(c.Step)
	return c.Current
}

func (c *FakePacingClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.Slept = append(c.Slept, d)
	c.Current = c.Current.Add(d)
	return nil
}
