package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/doctor"
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

// settleThresholdCalibration records the gates cfg searches with, unless
// they were set for another embedding model and not changed since: then
// that record is kept and returned, for the caller to warn about.
func settleThresholdCalibration(ctx context.Context, cfg config.Config, store storage.EventStore) (storage.ThresholdCalibration, error) {
	index, supported := store.(storage.EmbeddingIndex)
	if !supported {
		return storage.ThresholdCalibration{}, nil
	}
	recorded, err := index.ThresholdCalibration(ctx)
	if err != nil {
		return storage.ThresholdCalibration{}, err
	}
	indexed, err := index.EmbeddingModel(ctx)
	if err != nil {
		return storage.ThresholdCalibration{}, err
	}
	current := doctor.ConfiguredCalibration(cfg)
	if kept := recorded.OrIndexedWith(indexed, current); kept.OutdatedFor(current) {
		return kept, index.RecordThresholdCalibration(ctx, kept)
	}
	if recorded == current {
		return storage.ThresholdCalibration{}, nil
	}
	return storage.ThresholdCalibration{}, index.RecordThresholdCalibration(ctx, current)
}

// thresholdAdvice tells how to recalibrate the gates set for calibrated
// when searching with current; `reindex` and `doctor` share it.
func thresholdAdvice(calibrated, current string, language Language) string {
	return fmt.Sprintf(language.pick(
		"`retrieval.max_distance` e `retrieval.max_best_distance` foram calibrados para %s e não valem para %s; rode a calibração (`EMBEDDING_MODEL=<caminho de %s> go tool mage evalRetrieval`, no repositório) e ajuste os dois",
		"`retrieval.max_distance` and `retrieval.max_best_distance` were calibrated for %s and do not hold for %s; run the calibration (`EMBEDDING_MODEL=<path to %s> go tool mage evalRetrieval`, in the repository) and adjust both"),
		calibrated, current, current)
}

func (env commandEnv) warnPendingReindex(ctx context.Context, index storage.EmbeddingIndex) error {
	pending, err := index.ReindexPending(ctx)
	if err == nil && pending {
		fmt.Fprintln(env.stderr, env.language.pick("Reindexação incompleta: parte dos eventos ainda não tem vetor. Rode `cade reindex` para terminar.",
			"Unfinished reindex: some events have no vector yet. Run `cade reindex` to finish it."))
	}
	return err
}
