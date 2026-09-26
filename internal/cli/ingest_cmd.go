package cli

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/config"
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
		return fmt.Errorf("informe a fonte: cade ingest <git|browser|file|all> [ALVO...]")
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	jobs, err := planIngestJobs(env.toolkit.Sources(cfg), args[0], args[1:])
	if err != nil {
		return err
	}
	return env.withIngestPipeline(ctx, func(pipeline *ingest.Pipeline) error {
		return runIngestJobs(ctx, env, pipeline, jobs)
	})
}

// planIngestJobs resolves targets before any model is loaded, so a typo
// fails fast instead of after a multi-second model load.
func planIngestJobs(specs []ingest.SourceSpec, sourceName string, targets []string) ([]ingestJob, error) {
	if sourceName == allSourcesKeyword {
		return defaultJobsForAll(specs)
	}
	spec, err := ingest.FindSource(specs, sourceName)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		targets = spec.DefaultTargets
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("nenhum alvo para a fonte %q: passe um caminho ou configure-o em sources no arquivo de configuração", sourceName)
	}
	return jobsFor(spec, targets), nil
}

func defaultJobsForAll(specs []ingest.SourceSpec) ([]ingestJob, error) {
	var jobs []ingestJob
	for _, spec := range specs {
		jobs = append(jobs, jobsFor(spec, spec.DefaultTargets)...)
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("nenhuma fonte configurada; edite sources no arquivo de configuração (cade init)")
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
		embedder, err := env.toolkit.LoadEmbedder(cfg.Embedding)
		if err != nil {
			return err
		}
		defer embedder.Close()
		return use(ingest.NewPipeline(store, embedder, cfg.Embedding.DocumentPrefix, env.logger))
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
			return fmt.Errorf("ingest %s %q (os eventos já gravados foram mantidos; rode novamente para continuar): %w", job.spec.Name, job.target, err)
		}
	}
	return nil
}

func printIngestReport(env commandEnv, job ingestJob, report ingest.Report) {
	fmt.Fprintf(env.stdout, "%-8s %s: %d novos, %d atualizados, %d já existentes (%d lidos)\n",
		job.spec.Name, job.target, report.Inserted, report.Updated, report.AlreadyStored, report.Collected)
}
