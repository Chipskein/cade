package retrievalsuite

import (
	"fmt"
	"sort"

	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
)

// RelevanceScorer rates how well document answers query; higher is closer.
// llamacpp.Reranker implements it.
type RelevanceScorer interface {
	Score(query, document string) (float32, error)
}

// Reranking reorders the retrieved candidates before they are cut to Keep,
// the phase 17 experiment: retrieve more with the hybrid search, let a
// cross-encoder pick the order.
type Reranking struct {
	Scorer RelevanceScorer
	Keep   int
}

// Apply scores every hit against question by the text the model would
// read and returns the best Keep, best first.
//
//	hits, err := Reranking{Scorer: reranker, Keep: 6}.Apply("o que fiz no PROJ-481?", candidates)
func (r Reranking) Apply(question string, hits []storage.ScoredEvent) ([]storage.ScoredEvent, error) {
	scores := make(map[string]float32, len(hits))
	for _, hit := range hits {
		score, err := r.Scorer.Score(question, rag.PromptEvidenceText(hit))
		if err != nil {
			return nil, fmt.Errorf("rerank event %q for %q: %w", hit.Event.UID, question, err)
		}
		scores[hit.Event.UID] = score
	}
	ranked := append([]storage.ScoredEvent(nil), hits...)
	sort.SliceStable(ranked, func(i, j int) bool { return scores[ranked[i].Event.UID] > scores[ranked[j].Event.UID] })
	return ranked[:min(len(ranked), r.Keep)], nil
}
