package retrievalsuite

import (
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// keywordScorer scores a document by whether it contains its keyword.
type keywordScorer struct {
	keyword string
	fail    error
}

func (s keywordScorer) Score(_, document string) (float32, error) {
	if strings.Contains(document, s.keyword) {
		return 1, s.fail
	}
	return 0, s.fail
}

func scoredHit(uid, content string) storage.ScoredEvent {
	return storage.ScoredEvent{Event: event.Event{UID: uid, Source: event.SourceGit, Content: content}}
}

func TestRerankingPutsTheBestScoredFirstAndCuts(t *testing.T) {
	hits := []storage.ScoredEvent{scoredHit("a", "cache"), scoredHit("b", "fila"), scoredHit("c", "redis cache")}
	got, err := Reranking{Scorer: keywordScorer{keyword: "redis"}, Keep: 2}.Apply("redis?", hits)
	if err != nil || len(got) != 2 || got[0].Event.UID != "c" || got[1].Event.UID != "a" {
		t.Fatalf("expected [c a], got %+v: %v", got, err)
	}
}

func TestRerankingReportsTheEventItCouldNotScore(t *testing.T) {
	_, err := Reranking{Scorer: keywordScorer{fail: errors.New("decode failed")}, Keep: 1}.Apply("q", []storage.ScoredEvent{scoredHit("a", "x")})
	if err == nil || !strings.Contains(err.Error(), `"a"`) {
		t.Fatalf("expected an error naming event a, got %v", err)
	}
}
