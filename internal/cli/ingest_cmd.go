package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingestrun"
	"github.com/chipskein/cade/internal/storage"
)

const allSourcesKeyword = "all"

// ingestJob is one (source, target) pair to ingest.
type ingestJob struct {
	spec   ingest.SourceSpec
	target string
}

// runIngest runs `cade ingest <fonte|all>` in this terminal, or one of
// the subcommands that control a run from any terminal (issue #41).
func runIngest(ctx context.Context, env commandEnv, args []string) error {
	if len(args) > 0 {
		if control, found := ingestSubcommands()[args[0]]; found {
			return control(ctx, env, args[1:])
		}
	}
	gentle, targets, err := parseForegroundIngestFlags(env, args)
	if err != nil {
		return err
	}
	detached := env.toolkit.IngestRuns.Detached
	return runRecordedIngest(ctx, env, targets, ingestrun.Mode{Background: detached, Gentle: gentle || detached})
}

// ingestPlan is what one run ingests, resolved before any model loads.
type ingestPlan struct {
	jobs []ingestJob
	// The file collectors read captions when created, after
	// describeImages has filled it.
	captions ingest.ImageCaptions
}

func (env commandEnv) planIngest(args []string) (ingestPlan, error) {
	if len(args) == 0 {
		return ingestPlan{}, errors.New(env.language.pick("informe a fonte: cade ingest <git|browser|file|teams|all> [ALVO...]", "name the source: cade ingest <git|browser|file|teams|all> [TARGET...]"))
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return ingestPlan{}, err
	}
	captions := ingest.ImageCaptions{}
	jobs, err := planIngestJobs(env.toolkit.Sources(cfg, captions), args[0], args[1:], env.language)
	return ingestPlan{jobs: jobs, captions: captions}, err
}

// runRecordedIngest runs args in this process, recorded in the state file
// once planned, so a typo never replaces the last run's state.
func runRecordedIngest(ctx context.Context, env commandEnv, args []string, mode ingestrun.Mode) error {
	plan, err := env.planIngest(args)
	if err != nil {
		return err
	}
	release, err := env.acquireIngestLock()
	if err != nil {
		return err
	}
	defer release()
	env.ingestRun = newIngestRun(env, args, mode)
	env.ingestRun.start()
	env.lowerPriorityIfGentle()
	err = env.executeIngest(ctx, plan)
	env.ingestRun.finish(err)
	return err
}

func (env commandEnv) executeIngest(ctx context.Context, plan ingestPlan) error {
	describe := func(cfg config.Config, store storage.EventStore) error {
		return env.describeImages(ctx, cfg, store, plan.jobs, plan.captions)
	}
	return env.withIngestPipeline(ctx, describe, func(pipeline *ingest.Pipeline) error {
		return runIngestJobs(ctx, env, pipeline, plan.jobs)
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
// all jobs of this run (RNF5.2). prepare runs before the embedder loads.
func (env commandEnv) withIngestPipeline(ctx context.Context, prepare func(config.Config, storage.EventStore) error, use func(*ingest.Pipeline) error) error {
	return env.withStore(ctx, func(cfg config.Config, store storage.EventStore) error {
		if err := env.checkEmbeddingModel(ctx, cfg, store); err != nil {
			return err
		}
		// Keeps the record current; `reindex` and `doctor` warn about it.
		if _, err := settleThresholdCalibration(ctx, cfg, store); err != nil {
			return err
		}
		if err := prepare(cfg, store); err != nil {
			return err
		}
		embedder, err := env.loadIngestEmbedder(ctx, cfg)
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

// loadIngestEmbedder loads the embedder, paced in gentle runs.
func (env commandEnv) loadIngestEmbedder(ctx context.Context, cfg config.Config) (ClosableEmbedder, error) {
	env.ingestRun.update(ingestrun.StageLoadingEmbedder, func() string {
		return env.language.pick("Carregando modelo de embedding…", "Loading embedding model…")
	})
	loaded, err := env.toolkit.LoadEmbedder(cfg.Embedding, env.logger)
	if err != nil {
		return nil, err
	}
	return env.ingestRun.paceEmbedder(ctx, cfg, loaded), nil
}

func runIngestJobs(ctx context.Context, env commandEnv, pipeline *ingest.Pipeline, jobs []ingestJob) error {
	for i, job := range jobs {
		collector, err := job.spec.NewCollector(job.target)
		if err != nil {
			return err
		}
		label := job.spec.Name + " " + job.target
		env.ingestRun.enterJob(i+1, len(jobs), label)
		progress := newIngestProgress(env, label, estimateEvents(ctx, env, collector))
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

// estimateEvents is the collector's event count for the ETA, 0 when it
// cannot tell cheaply; a failed estimate only costs the ETA.
func estimateEvents(ctx context.Context, env commandEnv, collector ingest.EventCollector) int {
	estimator, estimates := collector.(ingest.EventEstimator)
	if !estimates {
		return 0
	}
	total, err := estimator.EstimateEvents(ctx)
	if err != nil {
		env.logger.Debug("no event estimate for the ETA", "error", err.Error())
		return 0
	}
	return total
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
