package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

// recaptionImages describes again the images whose description another
// model or prompt version wrote: `cade reindex --captions`. It works in
// batches of ingest.max_images_per_run, freeing the vision model before
// loading the embedder each time, so the two never add up in memory; an
// interrupted run resumes, since updated images are no longer outdated.
func (env commandEnv) recaptionImages(ctx context.Context) error {
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		index, supported := store.(storage.ImageCaptionIndex)
		if !supported {
			return errors.New(env.language.pick("este banco não guarda descrições de imagens", "this database does not keep image descriptions"))
		}
		if err := env.checkEmbeddingModel(ctx, cfg, store); err != nil {
			return err
		}
		outdated, err := index.OutdatedImages(ctx, cfg.Generation.ModelName(), imagecaption.PromptVersion)
		if err != nil {
			return err
		}
		tally, err := env.recaptionInBatches(ctx, cfg, store, outdated)
		fmt.Fprintln(env.stdout, recaptionLine(tally, cfg.Generation.ModelName(), env.language))
		return err
	})
}

// recaptionTally counts a `reindex --captions` run.
type recaptionTally struct {
	recaptioned int
	skipped     int
}

func (env commandEnv) recaptionInBatches(ctx context.Context, cfg config.Config, store storage.EventStore, outdated []event.Event) (recaptionTally, error) {
	var tally recaptionTally
	status := statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal}
	defer status.clear()
	batchSize := max(cfg.Ingest.MaxImagesPerRun, 1)
	for start := 0; start < len(outdated); start += batchSize {
		batch := outdated[start:min(start+batchSize, len(outdated))]
		updated, err := env.recaptionBatch(ctx, cfg, batch, recaptionProgress(&status, start, len(outdated), env.language))
		tally.skipped += len(batch) - len(updated)
		if err != nil {
			return tally, err
		}
		if err := env.storeRecaptioned(ctx, cfg, store, updated); err != nil {
			return tally, err
		}
		tally.recaptioned += len(updated)
	}
	return tally, nil
}

// recaptionProgress shows "Descrevendo de novo: 3/120 imagens" for a batch
// starting at offset.
func recaptionProgress(status *statusLine, offset, total int, language Language) func(done int) {
	return func(done int) {
		status.show(fmt.Sprintf(language.pick("Descrevendo de novo: %d/%d imagens", "Describing again: %d/%d images"), offset+done, total))
	}
}

// recaptionBatch describes batch with the vision model, freed on return.
func (env commandEnv) recaptionBatch(ctx context.Context, cfg config.Config, batch []event.Event, progress func(done int)) ([]event.Event, error) {
	recaptioner := imagecaption.NewRecaptioner(env.toolkit.RootFS, env.describerLoader(cfg), cfg.Generation.ModelName(), env.logger)
	defer recaptioner.Close()
	var updated []event.Event
	for i, stored := range batch {
		recaptioned, ok, err := recaptioner.Recaption(ctx, stored)
		if err != nil {
			return updated, err
		}
		if ok {
			updated = append(updated, recaptioned)
		}
		progress(i + 1)
	}
	return updated, recaptioner.Close()
}

// storeRecaptioned embeds and stores the new descriptions, with the
// embedder loaded only for them.
func (env commandEnv) storeRecaptioned(ctx context.Context, cfg config.Config, store storage.EventStore, updated []event.Event) error {
	if len(updated) == 0 {
		return nil
	}
	embedder, err := env.toolkit.LoadEmbedder(cfg.Embedding, env.logger)
	if err != nil {
		return err
	}
	defer embedder.Close()
	pipeline := ingest.NewPipeline(store, embedder, cfg.Embedding.DocumentPrefix, env.logger).WithRedaction(cfg.Ingest.Redact)
	for _, ev := range updated {
		if err := pipeline.ReplaceStored(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

var (
	recaptionedNoun      = nounForms{"imagem descrita de novo", "imagens descritas de novo", "image described again", "images described again"}
	skippedRecaptionNoun = nounForms{"pulada", "puladas", "skipped", "skipped"}
)

// recaptionLine is "3 imagens descritas de novo com M (1 pulada: …)".
func recaptionLine(tally recaptionTally, model string, language Language) string {
	line := fmt.Sprintf(language.pick("%s com %s", "%s with %s"), language.count(tally.recaptioned, recaptionedNoun), model)
	if tally.skipped == 0 {
		return line + "."
	}
	return line + fmt.Sprintf(language.pick(" (%s: o arquivo sumiu ou mudou; o próximo `cade ingest file` cuida dele).",
		" (%s: the file is gone or changed; the next `cade ingest file` handles it)."), language.count(tally.skipped, skippedRecaptionNoun))
}
