package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

// runReindex recomputes every vector with the configured embedding model:
// `cade reindex`. Changing the model no longer needs a new database, and
// an interrupted run resumes.
func runReindex(ctx context.Context, env commandEnv, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("cade reindex não recebe argumentos, recebido %q", args)
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		index, supported := store.(storage.EmbeddingIndex)
		if !supported {
			return fmt.Errorf("este banco não suporta reindexação")
		}
		embedder, err := env.toolkit.LoadEmbedder(cfg.Embedding, env.logger)
		if err != nil {
			return err
		}
		defer embedder.Close()
		pipeline := ingest.NewPipeline(store, embedder, cfg.Embedding.DocumentPrefix, env.logger)
		return env.reindexWith(ctx, pipeline, index, cfg.Embedding.ModelName())
	})
}

func (env commandEnv) reindexWith(ctx context.Context, pipeline *ingest.Pipeline, index storage.EmbeddingIndex, model string) error {
	status := statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal}
	done, err := pipeline.Reindex(ctx, index, model, reindexProgress(&status))
	status.clear()
	if err != nil {
		return fmt.Errorf("reindex parou após %d eventos (rode `cade reindex` de novo para continuar): %w", done, err)
	}
	fmt.Fprintf(env.stdout, "%d eventos reindexados com %s.\n", done, model)
	return nil
}

// reindexProgress redraws on a terminal and logs every 10% otherwise.
func reindexProgress(status *statusLine) ingest.ReindexProgress {
	lastDecile := -1
	return func(done, total int) {
		decile := done * 10 / max(total, 1)
		if !status.interactive && decile == lastDecile {
			return
		}
		lastDecile = decile
		status.show(fmt.Sprintf("Reindexando: %d/%d eventos (%d%%)", done, total, done*100/max(total, 1)))
	}
}
