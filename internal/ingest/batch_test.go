package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
)

// SteppingClock advances by Step on every reading.
type SteppingClock struct {
	At   time.Time
	Step time.Duration
}

func (c *SteppingClock) Now() time.Time {
	c.At = c.At.Add(c.Step)
	return c.At
}

func writeOneEvent(t *testing.T, batches *eventBatcher) error {
	t.Helper()
	_, err := batches.writer(context.Background())
	testcheck.NoError(t, err)
	return batches.eventWritten()
}

func TestEventBatcherCommitsABatchOlderThanMaxAge(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	clock := &SteppingClock{Step: batchMaxAge}
	batches := newEventBatcher(store, clock.Now, eventsPerCommit)
	testcheck.NoError(t, writeOneEvent(t, batches))
	if store.Commits != 1 || batches.open != nil {
		t.Fatalf("expected a batch open for %s committed after one event, got %d commits", batchMaxAge, store.Commits)
	}
}

func TestEventBatcherKeepsAYoungBatchUnderItsSizeOpen(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	batches := newEventBatcher(store, (&SteppingClock{}).Now, 2)
	testcheck.NoError(t, writeOneEvent(t, batches))
	if store.Commits != 0 || batches.open == nil {
		t.Fatalf("expected the batch still open after 1 of 2 events, got %d commits", store.Commits)
	}
}

func TestEventBatcherRollsBackAFailedCommit(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	batches := newEventBatcher(store, (&SteppingClock{}).Now, 1)
	store.FailWith = errors.New("disk full")
	if err := writeOneEvent(t, batches); !errors.Is(err, store.FailWith) || store.Rollbacks != 1 {
		t.Fatalf("expected the failed commit rolled back, got err %v and %d rollbacks", err, store.Rollbacks)
	}
}

func TestEventBatcherFinishWithoutABatchReturnsTheCollectionError(t *testing.T) {
	collectErr := errors.New("collector broke")
	batches := newEventBatcher(testfakes.NewFakeEventStore(), time.Now, 2)
	if err := batches.finish(collectErr); !errors.Is(err, collectErr) {
		t.Fatalf("expected %v, got %v", collectErr, err)
	}
	testcheck.NoError(t, batches.finish(nil))
}
