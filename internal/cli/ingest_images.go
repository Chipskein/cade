package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/imagecaption"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/rootfs"
	"github.com/chipskein/cade/internal/storage"
)

// fileSourceName is the source whose directories hold the images.
const fileSourceName = "file"

// describeImages is the first stage of `ingest` with images on (phase 19):
// it plans the images of the file jobs into captions, loading the vision
// model only if one needs describing, and frees it before the embedder
// loads, so the two never add up in memory.
func (env commandEnv) describeImages(ctx context.Context, cfg config.Config, store storage.EventStore, jobs []ingestJob, captions ingest.ImageCaptions) error {
	roots := env.imageRoots(jobs)
	if !cfg.Sources.Images || len(roots) == 0 {
		return nil
	}
	index, supported := store.(imagecaption.Store)
	if !supported {
		return errors.New(env.language.pick("este banco não guarda descrições de imagens", "this database does not keep image descriptions"))
	}
	planner := imagecaption.NewPlanner(index, env.describerLoader(cfg), imageSettings(cfg), env.logger)
	defer planner.Close()
	if err := env.planImages(ctx, planner, roots, cfg.Sources); err != nil {
		return err
	}
	maps.Copy(captions, planner.Captions())
	printImageReport(env, planner.Tally())
	return planner.Close()
}

// imageRoots are the absolute directories of the file jobs that exist; a
// missing one is reported by its collector, as without images.
func (env commandEnv) imageRoots(jobs []ingestJob) []string {
	var roots []string
	for _, job := range jobs {
		root, err := filepath.Abs(job.target)
		if job.spec.Name != fileSourceName || err != nil {
			continue
		}
		if info, err := fs.Stat(env.toolkit.RootFS, rootfs.Name(root)); err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	return roots
}

func (env commandEnv) planImages(ctx context.Context, planner *imagecaption.Planner, roots []string, sources config.SourcesConfig) error {
	status := statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal}
	defer status.clear()
	planner.WithProgress(func(tally imagecaption.Tally) {
		status.show(env.language.pick("Descrevendo imagens: ", "Describing images: ") + imageTally(tally, env.language))
	})
	for _, root := range roots {
		files, err := fs.Sub(env.toolkit.RootFS, rootfs.Name(root))
		if err != nil {
			return fmt.Errorf("open directory %q: %w", root, err)
		}
		if err := planner.PlanDirectory(ctx, files, root, filesource.OptionsFor(sources, nil)); err != nil {
			return err
		}
	}
	return nil
}

func (env commandEnv) describerLoader(cfg config.Config) imagecaption.DescriberLoader {
	return func() (imagecaption.ClosableDescriber, error) {
		return env.toolkit.LoadImageDescriber(cfg.Generation, cfg.Vision, env.logger)
	}
}

func imageSettings(cfg config.Config) imagecaption.Settings {
	return imagecaption.Settings{MaxImageBytes: cfg.Ingest.MaxImageBytes, MaxImagesPerRun: cfg.Ingest.MaxImagesPerRun,
		Model: cfg.Generation.ModelName()}
}

// Counted phrases of the image report; the noun, "imagens", is implied.
var (
	describedNoun  = nounForms{"descrita", "descritas", "described", "described"}
	reusedNoun     = nounForms{"reaproveitada", "reaproveitadas", "reused", "reused"}
	pendingNoun    = nounForms{"para depois", "para depois", "left for later", "left for later"}
	unreadableNoun = nounForms{"ilegível", "ilegíveis", "unreadable", "unreadable"}
)

// imageTally is "3 descritas, 1 reaproveitada, 45 para depois, 0 ilegíveis".
func imageTally(tally imagecaption.Tally, language Language) string {
	return language.count(tally.Described, describedNoun) + ", " + language.count(tally.Reused, reusedNoun) + ", " +
		language.count(tally.Pending, pendingNoun) + ", " + language.count(tally.Unreadable, unreadableNoun)
}

// printImageReport prints the tally when there was anything to do, and how
// to go on when images were left for later.
func printImageReport(env commandEnv, tally imagecaption.Tally) {
	if tally == (imagecaption.Tally{}) {
		return
	}
	fmt.Fprintf(env.stdout, "%-8s %s\n", env.language.pick("imagens", "images"), imageTally(tally, env.language))
	if tally.Pending > 0 {
		fmt.Fprintln(env.stdout, env.language.pick("         as imagens para depois são descritas nas próximas execuções de `cade ingest file`",
			"         images left for later are described by the next runs of `cade ingest file`"))
	}
}
