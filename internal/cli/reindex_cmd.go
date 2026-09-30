package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

// runReindex recomputes every vector with the configured embedding model:
// `cade reindex`; with --captions, it describes images again instead. Changing the model no longer needs a new database, and
// an interrupted run resumes.
func runReindex(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("reindex", env.stderr, env.language)
	captions := flags.Bool("captions", false, env.language.pick("descreve de novo as imagens cuja descrição veio de outro modelo ou prompt",
		"describes again the images whose description another model or prompt wrote"))
	positional, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return fmt.Errorf(env.language.pick("cade reindex não recebe argumentos, recebido %q", "cade reindex takes no arguments, got %q"), positional)
	}
	if *captions {
		return env.recaptionImages(ctx)
	}
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		index, supported := store.(storage.EmbeddingIndex)
		if !supported {
			return errors.New(env.language.pick("este banco não suporta reindexação", "this database does not support reindexing"))
		}
		outdated, err := settleThresholdCalibration(ctx, cfg, store)
		if err != nil {
			return err
		}
		embedder, err := env.toolkit.LoadEmbedder(cfg.Embedding, env.logger)
		if err != nil {
			return err
		}
		defer embedder.Close()
		pipeline := ingest.NewPipeline(store, embedder, cfg.Embedding.DocumentPrefix, env.logger)
		if outdated.Model != "" {
			fmt.Fprintf(env.stderr, env.language.pick("Aviso: %s.\n", "Warning: %s.\n"), thresholdAdvice(outdated.Model, cfg.Embedding.ModelName(), env.language))
		}
		return env.reindexWith(ctx, pipeline, index, cfg.Embedding.ModelName())
	})
}

func (env commandEnv) reindexWith(ctx context.Context, pipeline *ingest.Pipeline, index storage.EmbeddingIndex, model string) error {
	status := statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal}
	done, err := pipeline.Reindex(ctx, index, model, reindexProgress(&status, env.language))
	status.clear()
	if err != nil {
		return reindexStopped(done, err, env.language)
	}
	fmt.Fprintf(env.stdout, env.language.pick("%s com %s.\n", "%s with %s.\n"), env.language.count(done, reindexedNoun), model)
	return nil
}

func reindexStopped(done int, err error, language Language) error {
	return fmt.Errorf(language.pick("reindex parou após %s (rode `cade reindex` de novo para continuar): %w",
		"reindex stopped after %s (run `cade reindex` again to continue): %w"), language.count(done, eventNoun), err)
}

var reindexedNoun = nounForms{"evento reindexado", "eventos reindexados", "event reindexed", "events reindexed"}

// reindexProgress redraws on a terminal and logs every 10% otherwise.
func reindexProgress(status *statusLine, language Language) ingest.ReindexProgress {
	lastDecile := -1
	return func(done, total int) {
		decile := done * 10 / max(total, 1)
		if !status.interactive && decile == lastDecile {
			return
		}
		lastDecile = decile
		status.show(reindexProgressLine(done, total, language))
	}
}

// reindexProgressLine is "Reindexando: 3/10 eventos (30%)".
func reindexProgressLine(done, total int, language Language) string {
	return fmt.Sprintf(language.pick("Reindexando: %d/%s (%d%%)", "Reindexing: %d/%s (%d%%)"), done, language.count(total, eventNoun), done*100/max(total, 1))
}
