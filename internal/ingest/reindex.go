package ingest

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/storage"
)

// reindexBatch events are embedded and saved per transaction: an
// interruption loses at most one batch.
const reindexBatch = 128

// ReindexProgress observes a reindex: done so far out of total.
type ReindexProgress func(done, total int)

// Reindex recomputes every vector with this pipeline's embedder, recorded
// as model. A rebuild interrupted earlier with the same model resumes;
// otherwise it starts over. Returns how many events were embedded.
//
//	done, err := pipeline.Reindex(ctx, store, "nomic-embed-text-v2-moe.Q4_K_M.gguf", nil)
func (p *Pipeline) Reindex(ctx context.Context, index storage.EmbeddingIndex, model string, progress ReindexProgress) (int, error) {
	if err := startOrResume(ctx, index, model); err != nil {
		return 0, err
	}
	total, err := index.CountEventsWithoutEmbedding(ctx)
	if err != nil {
		return 0, err
	}
	done := 0
	for done < total {
		embedded, err := p.reindexBatch(ctx, index)
		if err != nil || embedded == 0 {
			return done, err
		}
		done += embedded
		if progress != nil {
			progress(done, total)
		}
	}
	return done, index.FinishReindex(ctx)
}

func startOrResume(ctx context.Context, index storage.EmbeddingIndex, model string) error {
	pending, err := index.ReindexPending(ctx)
	if err != nil {
		return err
	}
	current, err := index.EmbeddingModel(ctx)
	if err != nil {
		return err
	}
	if pending && current == model {
		return nil
	}
	return index.StartReindex(ctx, model)
}

func (p *Pipeline) reindexBatch(ctx context.Context, index storage.EmbeddingIndex) (int, error) {
	events, err := index.EventsWithoutEmbedding(ctx, reindexBatch)
	if err != nil || len(events) == 0 {
		return 0, err
	}
	embeddings := make([]storage.EventEmbedding, 0, len(events))
	for _, ev := range events {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		vector, err := p.embeddingFor(ev)
		if err != nil {
			return 0, err
		}
		embeddings = append(embeddings, storage.EventEmbedding{Event: ev, Vector: vector})
	}
	if err := index.SaveEmbeddings(ctx, embeddings); err != nil {
		return 0, fmt.Errorf("save %d reindexed embeddings: %w", len(embeddings), err)
	}
	return len(embeddings), nil
}
