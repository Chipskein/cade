package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/ingest/pdfplan"
	"github.com/chipskein/cade/internal/rootfs"
	"github.com/chipskein/cade/internal/storage"
)

// readPDFs is the stage of `ingest` with PDFs on (#108): it reads the PDFs of
// the file jobs into readings, before the embedder loads and the collectors
// run.
func (env commandEnv) readPDFs(ctx context.Context, cfg config.Config, store storage.EventStore, jobs []ingestJob, readings ingest.PDFReadings) error {
	roots := env.imageRoots(jobs)
	if !cfg.Sources.PDFs || len(roots) == 0 {
		return nil
	}
	index, supported := store.(pdfplan.Store)
	if !supported {
		return errors.New(env.language.pick("este banco não guarda o estado dos PDFs", "this database does not keep the state of PDFs"))
	}
	settings := pdfplan.Settings{MaxBytes: cfg.Ingest.MaxPDFBytes, MaxPages: cfg.Ingest.MaxPDFPages}
	planner := pdfplan.NewPlanner(index, env.toolkit.PDFReader, settings, env.logger)
	if err := env.planPDFFiles(ctx, planner, roots, cfg); err != nil {
		return err
	}
	maps.Copy(readings, planner.Readings())
	printPDFReport(env, planner.Tally())
	return nil
}

func (env commandEnv) planPDFFiles(ctx context.Context, planner *pdfplan.Planner, roots []string, cfg config.Config) error {
	for _, root := range roots {
		files, err := fs.Sub(env.toolkit.RootFS, rootfs.Name(root))
		if err != nil {
			return fmt.Errorf("open directory %q: %w", root, err)
		}
		if err := planner.PlanDirectory(ctx, files, root, filesource.OptionsFor(cfg.Sources, nil)); err != nil {
			return err
		}
	}
	return nil
}

// Counted phrases of the PDF report.
var (
	pdfReadNoun    = nounForms{"lido", "lidos", "read", "read"}
	pdfWaitingNoun = nounForms{"com páginas à espera", "com páginas à espera", "with pages waiting", "with pages waiting"}
)

// printPDFReport never names a file: the list could be long, and the debug log
// says which ones and why.
func printPDFReport(env commandEnv, tally pdfplan.Tally) {
	if tally == (pdfplan.Tally{}) {
		return
	}
	language := env.language
	fmt.Fprintf(env.stdout, "%-8s %s, %s, %s\n", "PDFs", language.count(tally.Read, pdfReadNoun),
		language.count(tally.Pending, pdfWaitingNoun), language.count(tally.Unreadable, unreadableNoun))
}
