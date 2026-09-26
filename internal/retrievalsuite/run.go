package retrievalsuite

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/rag"
	"github.com/chipskein/cade/internal/storage"
)

// Dependencies are the real parts under measurement.
type Dependencies struct {
	// Store must start empty; the corpus is ingested into it.
	Store          storage.EventStore
	Embedder       llm.Embedder
	Settings       rag.Settings
	DocumentPrefix string
	Logger         *slog.Logger
}

// CaseDone observes a run; done counts from 1.
type CaseDone func(done, total int, result CaseResult)

// Run ingests the corpus with the ingestion pipeline and retrieves every
// question as `cade ask` would, without generating answers.
//
//	board, err := retrievalsuite.Run(ctx, deps, suite, nil)
func Run(ctx context.Context, deps Dependencies, suite Suite, onCase CaseDone) (Scoreboard, error) {
	pipeline := ingest.NewPipeline(deps.Store, deps.Embedder, deps.DocumentPrefix, deps.Logger)
	if _, err := pipeline.Run(ctx, corpusCollector{events: suite.events()}, nil); err != nil {
		return Scoreboard{}, fmt.Errorf("ingest corpus of %d events: %w", len(suite.Events), err)
	}
	answerer := rag.NewAnswerer(rag.Dependencies{Store: deps.Store, Embedder: deps.Embedder,
		Now: func() time.Time { return suite.Now }, Logger: deps.Logger}, deps.Settings)
	var board Scoreboard
	for i, suiteCase := range suite.Cases {
		hits, err := answerer.Retrieve(ctx, suiteCase.Query(suite.Now), rag.AnswerObserver{})
		if err != nil {
			return Scoreboard{}, fmt.Errorf("retrieve %q: %w", suiteCase.Question, err)
		}
		result := CaseResult{Question: suiteCase.Question, Relevant: suiteCase.Relevant, Retrieved: hitIDs(hits)}
		board.Results = append(board.Results, result)
		if onCase != nil {
			onCase(i+1, len(suite.Cases), result)
		}
	}
	return board, nil
}

func hitIDs(hits []storage.ScoredEvent) []string {
	ids := make([]string, len(hits))
	for i, hit := range hits {
		ids[i] = hit.Event.UID
	}
	return ids
}

// corpusCollector feeds the corpus to the ingestion pipeline.
type corpusCollector struct {
	events []event.Event
}

func (c corpusCollector) CollectEvents(_ context.Context, emit ingest.EmitFunc) error {
	for _, ev := range c.events {
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}
