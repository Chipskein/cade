package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
)

// checkEmbeddingModel refuses to mix vectors of two models: same-dimension
// models (nomic v1.5 and v2-moe are both 768) would otherwise be compared
// silently and search would return noise. A database without a recorded
// model (created before this check) adopts the configured one.
func (env commandEnv) checkEmbeddingModel(ctx context.Context, cfg config.Config, store storage.EventStore) error {
	index, supported := store.(storage.EmbeddingIndex)
	if !supported {
		return nil
	}
	configured := cfg.Embedding.ModelName()
	stored, err := index.EmbeddingModel(ctx)
	if err != nil {
		return err
	}
	if stored == "" {
		return index.RecordEmbeddingModel(ctx, configured)
	}
	if stored != configured {
		return fmt.Errorf(env.language.pick("o banco foi indexado com %q e a configuração usa %q; rode `cade reindex` para recalcular os vetores com o novo modelo",
			"the database was indexed with %q and the config uses %q; run `cade reindex` to recompute the vectors with the new model"), stored, configured)
	}
	return env.warnPendingReindex(ctx, index)
}

func (env commandEnv) warnPendingReindex(ctx context.Context, index storage.EmbeddingIndex) error {
	pending, err := index.ReindexPending(ctx)
	if err == nil && pending {
		fmt.Fprintln(env.stderr, env.language.pick("Reindexação incompleta: parte dos eventos ainda não tem vetor. Rode `cade reindex` para terminar.",
			"Unfinished reindex: some events have no vector yet. Run `cade reindex` to finish it."))
	}
	return err
}
