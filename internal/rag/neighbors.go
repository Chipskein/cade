package rag

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/textnorm"
	"github.com/chipskein/cade/internal/timeline"
)

// "o que eu estava lendo quando corrigi o webhook?" names the commit, not
// the page: the page shares no words with the question, so no search finds
// it. Its time does (#21, step 1): the closest hit brings the events next
// to it, within the window `cade timeline --around` uses.

// maxTimeNeighbors caps the events one question adds to the evidence: the
// closest in time come first, and more would crowd the prompt.
const maxTimeNeighbors = 4

// timeContextCue matches a question about what happened at the time of
// something else; without one, answers are left as they were. "quando" and
// "when" count only after the first word: "quando é o deploy?" asks for a
// date, and a neighbor would be evidence nobody searched for.
var timeContextCue = regexp.MustCompile(`\S\s+(quando|when)\b|\b(enquanto|antes|depois|while|before|after)\b`)

func asksForTimeContext(question string) bool {
	return timeContextCue.MatchString(textnorm.Fold(question))
}

// withTimeNeighbors appends the closest hit's neighbors in time to hits,
// for a question that asks for them. A neighbor keeps the question's
// source and own-commit filters, and an event already among the hits is
// not repeated.
func (a *Answerer) withTimeNeighbors(ctx context.Context, query queryplan.Query, hits []storage.ScoredEvent) ([]storage.ScoredEvent, error) {
	if len(hits) == 0 || !asksForTimeContext(query.Question) {
		return hits, nil
	}
	anchor := hits[0]
	around, err := a.store.EventsBetween(ctx, anchor.Event.Timestamp.Add(-timeline.DefaultAroundWindow), anchor.Event.Timestamp.Add(timeline.DefaultAroundWindow))
	if err != nil {
		return nil, fmt.Errorf("load events around %q: %w", anchor.Event.UID, err)
	}
	for _, neighbor := range closestInTime(neighborCandidates(around, hits, query), anchor.Event.Timestamp) {
		hits = append(hits, storage.ScoredEvent{Event: neighbor, Distance: anchor.Distance, LatestAt: neighbor.Timestamp})
	}
	a.logger.Debug("time neighbors", "anchor", anchor.Event.UID, "evidence_count", len(hits))
	return hits, nil
}

// neighborCandidates drops what the hits already hold and what the query
// filters out.
func neighborCandidates(around []event.Event, hits []storage.ScoredEvent, query queryplan.Query) []event.Event {
	seen := map[string]bool{}
	for _, hit := range hits {
		seen[repeatKey(hit.Event)] = true
	}
	var candidates []event.Event
	for _, ev := range around {
		key := repeatKey(ev)
		if seen[key] || query.Source != "" && ev.Source != query.Source || query.OwnCommitsOnly && ev.IsOthersCommit() {
			continue
		}
		seen[key] = true
		candidates = append(candidates, ev)
	}
	return candidates
}

// closestInTime keeps the maxTimeNeighbors events nearest to at.
func closestInTime(events []event.Event, at time.Time) []event.Event {
	sorted := slices.Clone(events)
	slices.SortStableFunc(sorted, func(x, y event.Event) int {
		return cmp.Compare(absDuration(x.Timestamp.Sub(at)), absDuration(y.Timestamp.Sub(at)))
	})
	return sorted[:min(len(sorted), maxTimeNeighbors)]
}

func absDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return -duration
	}
	return duration
}
