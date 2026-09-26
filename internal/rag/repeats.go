package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/provenance"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
)

// A page visited 15 times is 15 events with the same text and the same
// vector: they took every top_k slot and pushed the other evidence out
// (redundancy 0.17 on the retrieval suite). Answers keep one hit per
// thing the user would call the same; listings still show every visit.

// repeatKey is that thing: the page's URL, the file's path, the commit,
// the message (the provenance locator), within its source.
func repeatKey(ev event.Event) string {
	return string(ev.Source) + "|" + provenance.Of(ev).Locator
}

// collapseRepeats keeps the closest hit of each key, in order, counting
// the others and remembering the latest occurrence.
func collapseRepeats(hits []storage.ScoredEvent) []storage.ScoredEvent {
	var kept []storage.ScoredEvent
	position := map[string]int{}
	for _, hit := range hits {
		key := repeatKey(hit.Event)
		index, seen := position[key]
		if !seen {
			position[key] = len(kept)
			hit.LatestAt = hit.Event.Timestamp
			kept = append(kept, hit)
			continue
		}
		kept[index].Repeats++
		kept[index].LatestAt = later(kept[index].LatestAt, hit.Event.Timestamp)
	}
	return kept
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// maxSearchWidening bounds how far searchDistinct widens the search: a
// history dominated by one page stops at top_k × 64 neighbours.
const maxSearchWidening = 64

// searchDistinct asks the vector index for neighbours until top_k distinct
// things remain once chatter and repeats are dropped, or the index has
// nothing more to give.
func (a *Answerer) searchDistinct(ctx context.Context, embedding []float32, query queryplan.Query) ([]storage.ScoredEvent, error) {
	limit := a.settings.TopK * chatterHeadroom
	for {
		hits, err := a.store.SearchSimilar(ctx, similarityQuery(embedding, query, limit))
		if err != nil {
			return nil, err
		}
		distinct := collapseRepeats(withoutRemoved(withoutChatter(bestChunkPerEvent(hits))))
		if len(distinct) >= a.settings.TopK || len(hits) < limit || limit >= a.settings.TopK*maxSearchWidening {
			return distinct[:min(len(distinct), a.settings.TopK)], nil
		}
		limit *= 4
	}
}

var repeatNouns = map[event.Source]string{event.SourceBrowser: "visitas", event.SourceFile: "versões"}

// RepeatNote describes folded repeats for the prompt and the sources list,
// e.g. "12 visitas, última em 2026-09-25 14:10"; "" when there are none.
//
//	note := rag.RepeatNote(hit, time.Local)
func RepeatNote(hit storage.ScoredEvent, location *time.Location) string {
	if hit.Repeats == 0 {
		return ""
	}
	noun, known := repeatNouns[hit.Event.Source]
	if !known {
		noun = "ocorrências"
	}
	return fmt.Sprintf("%d %s, última em %s", hit.Repeats+1, noun, hit.LatestAt.In(location).Format(evidenceTimeLayout))
}

// withoutRemoved drops files that disappeared from their directory: the
// timeline keeps them as history, but an answer should not cite a note that
// no longer exists.
func withoutRemoved(hits []storage.ScoredEvent) []storage.ScoredEvent {
	var kept []storage.ScoredEvent
	for _, hit := range hits {
		if hit.Event.Source != event.SourceFile || hit.Event.File().RemovedAt.IsZero() {
			kept = append(kept, hit)
		}
	}
	return kept
}

// bestChunkPerEvent keeps each event's closest chunk: the vector index
// returns one hit per chunk, and two chunks of one note are not two
// sources (nor "2 versões").
func bestChunkPerEvent(hits []storage.ScoredEvent) []storage.ScoredEvent {
	seen := map[string]bool{}
	var kept []storage.ScoredEvent
	for _, hit := range hits {
		if !seen[hit.Event.UID] {
			seen[hit.Event.UID] = true
			kept = append(kept, hit)
		}
	}
	return kept
}
