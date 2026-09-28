package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/storage"
)

const allSourcesKeyword = "all"

// ingestJob is one (source, target) pair to ingest.
type ingestJob struct {
	spec   ingest.SourceSpec
	target string
}

func runIngest(ctx context.Context, env commandEnv, args []string) error {
	if len(args) == 0 {
		return errors.New(env.language.pick("informe a fonte: cade ingest <git|browser|file|teams|all> [ALVO...]", "name the source: cade ingest <git|browser|file|teams|all> [TARGET...]"))
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	jobs, err := planIngestJobs(env.toolkit.Sources(cfg), args[0], args[1:], env.language)
	if err != nil {
		return err
	}
	return env.withIngestPipeline(ctx, func(pipeline *ingest.Pipeline) error {
		return runIngestJobs(ctx, env, pipeline, jobs)
	})
}

// planIngestJobs resolves targets before any model is loaded, so a typo
// fails fast instead of after a multi-second model load.
func planIngestJobs(specs []ingest.SourceSpec, sourceName string, targets []string, language Language) ([]ingestJob, error) {
	if sourceName == allSourcesKeyword {
		return defaultJobsForAll(specs, language)
	}
	spec, err := ingest.FindSource(specs, sourceName)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		targets = spec.DefaultTargets
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf(language.pick("nenhum alvo para a fonte %q: passe um caminho ou configure-o em sources no arquivo de configuração",
			"no target for source %q: pass a path or set it under sources in the config file"), sourceName)
	}
	return jobsFor(spec, targets), nil
}

func defaultJobsForAll(specs []ingest.SourceSpec, language Language) ([]ingestJob, error) {
	var jobs []ingestJob
	for _, spec := range specs {
		jobs = append(jobs, jobsFor(spec, spec.DefaultTargets)...)
	}
	if len(jobs) == 0 {
		return nil, errors.New(language.pick("nenhuma fonte configurada; edite sources no arquivo de configuração (cade init)",
			"no source configured; edit sources in the config file (cade init)"))
	}
	return jobs, nil
}

func jobsFor(spec ingest.SourceSpec, targets []string) []ingestJob {
	jobs := make([]ingestJob, len(targets))
	for i, target := range targets {
		jobs[i] = ingestJob{spec: spec, target: config.ExpandHome(target)}
	}
	return jobs
}

// withIngestPipeline opens the store and loads the embedding model once for
// all jobs of this run (RNF5.2).
func (env commandEnv) withIngestPipeline(ctx context.Context, use func(*ingest.Pipeline) error) error {
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		if err := env.checkEmbeddingModel(ctx, cfg, store); err != nil {
			return err
		}
		embedder, err := env.toolkit.LoadEmbedder(cfg.Embedding, env.logger)
		if err != nil {
			return err
		}
		defer embedder.Close()
		retention := map[event.Source]int{}
		for source, days := range cfg.Ingest.Retention.MaxAgeDays {
			retention[event.Source(source)] = days
		}
		return use(ingest.NewPipeline(store, embedder, cfg.Embedding.DocumentPrefix, env.logger).WithRedaction(cfg.Ingest.Redact).WithRetention(retention))
	})
}

func runIngestJobs(ctx context.Context, env commandEnv, pipeline *ingest.Pipeline, jobs []ingestJob) error {
	for _, job := range jobs {
		collector, err := job.spec.NewCollector(job.target)
		if err != nil {
			return err
		}
		progress := newIngestProgress(env, job.spec.Name+" "+job.target)
		report, err := pipeline.Run(ctx, collector, progress.update)
		progress.finish()
		printIngestReport(env, job, report)
		if err != nil {
			return fmt.Errorf(env.language.pick("ingest %s %q (os eventos já gravados foram mantidos; rode novamente para continuar): %w",
				"ingest %s %q (events already stored were kept; run it again to continue): %w"), job.spec.Name, job.target, err)
		}
	}
	return nil
}

// Counted phrases of the ingestion report; the noun, "eventos", is implied.
var (
	insertedNoun      = nounForms{"novo", "novos", "new", "new"}
	updatedNoun       = nounForms{"atualizado", "atualizados", "updated", "updated"}
	alreadyStoredNoun = nounForms{"já existente", "já existentes", "already stored", "already stored"}
	removedAtNoun     = nounForms{"removido da origem", "removidos da origem", "removed at the source", "removed at the source"}
	collectedNoun     = nounForms{"lido", "lidos", "read", "read"}
)

func printIngestReport(env commandEnv, job ingestJob, report ingest.Report) {
	fmt.Fprintf(env.stdout, "%-8s %s\n", job.spec.Name, ingestReportLine(job.target, report, env.language))
}

// ingestReportLine is "alvo: 1 novo, 0 atualizados, 2 já existentes (3 lidos)".
func ingestReportLine(target string, report ingest.Report, language Language) string {
	removed := ""
	if report.Removed > 0 {
		removed = ", " + language.count(report.Removed, removedAtNoun)
	}
	return fmt.Sprintf("%s: %s%s (%s)", target, ingestTally(report, language), removed, language.count(report.Collected, collectedNoun))
}

// ingestTally is "1 novo, 0 atualizados, 2 já existentes", shared by the
// report and the progress line.
func ingestTally(report ingest.Report, language Language) string {
	return language.count(report.Inserted, insertedNoun) + ", " + language.count(report.Updated, updatedNoun) + ", " +
		language.count(report.AlreadyStored, alreadyStoredNoun)
}
