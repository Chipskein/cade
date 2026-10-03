package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/idbdiscovery"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/storage"
)

// percentScale turns an overlap ratio into the percentage printed.
const percentScale = 100

var (
	missingPathNoun = nounForms{"caminho sumiu", "caminhos sumiram", "path gone", "paths gone"}
	changedPathNoun = nounForms{"mudou de tipo", "mudaram de tipo", "changed kind", "changed kinds"}
)

// schemaChecker checks saved schemas against the IndexedDB they read and,
// when asked, regenerates the ones that drifted. It loads the model once,
// on the first regeneration.
type schemaChecker struct {
	env       commandEnv
	cfg       config.Config
	dir       idbmap.SchemaDir
	update    bool
	rekey     bool
	generator ClosableGenerator
}

// runSchemaCheck reports, per schema and directory, whether the application
// still matches its schema; it fails while a drift is left unresolved, so
// it can run on a timer.
func runSchemaCheck(ctx context.Context, env commandEnv, args []string) error {
	flags := newFlagSet("schema-check", env.stderr, env.language)
	update := flags.Bool("update", false, env.language.pick("regenera com o modelo local os schemas que mudaram", "regenerates with the local model the schemas that drifted"))
	rekey := flags.Bool("rekey", false, env.language.pick("com --update, aceita um schema que muda a identidade das mensagens e dá o UID novo às já indexadas (copia o banco antes)",
		"with --update, accepts a schema that changes the messages' identity and gives the indexed ones their new UID (copies the database first)"))
	names, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	checker := &schemaChecker{env: env, cfg: cfg, dir: idbmap.NewSchemaDir(env.toolkit.SchemaFiles, cfg.Sources.IndexedDBSchemaDir), update: *update, rekey: *rekey}
	defer checker.close()
	return checker.checkAll(ctx, names)
}

func (c *schemaChecker) checkAll(ctx context.Context, names []string) error {
	if len(names) == 0 {
		saved, err := c.dir.Names()
		if err != nil {
			return err
		}
		names = saved
	}
	unresolved := 0
	for _, name := range names {
		left, err := c.checkSchema(ctx, name)
		if err != nil {
			return err
		}
		unresolved += left
	}
	if unresolved > 0 {
		return fmt.Errorf(c.env.language.pick("%d schema(s) com mudança não resolvida", "%d schema(s) with an unresolved change"), unresolved)
	}
	return nil
}

// checkSchema checks every directory of one schema and returns how many
// still drift.
func (c *schemaChecker) checkSchema(ctx context.Context, name string) (int, error) {
	dirs := c.cfg.Sources.IndexedDBDirs[name]
	if len(dirs) == 0 {
		fmt.Fprintf(c.env.stdout, c.env.language.pick("%s: nenhum diretório em sources.indexeddb_dirs\n", "%s: no directory in sources.indexeddb_dirs\n"), name)
	}
	unresolved := 0
	for _, dir := range dirs {
		resolved, err := c.checkDirectory(ctx, name, dir)
		if err != nil {
			return 0, err
		}
		if !resolved {
			unresolved++
		}
	}
	return unresolved, nil
}

func (c *schemaChecker) checkDirectory(ctx context.Context, name, dir string) (bool, error) {
	schema, err := c.dir.Load(name)
	if err != nil {
		return false, err
	}
	records, err := c.env.toolkit.StoreReaders.ReadKind(schema.Records.Kind, config.ExpandHome(dir))
	if err != nil {
		return false, err
	}
	drift, err := idbmap.MeasureDrift(schema, records)
	if err != nil {
		return false, err
	}
	c.printDrift(name, dir, drift)
	if !drift.Drifted() || !c.update {
		return !drift.Drifted(), nil
	}
	return c.regenerate(ctx, schema, records, drift)
}

func (c *schemaChecker) printDrift(name, dir string, drift idbmap.Drift) {
	language := c.env.language
	prefix := fmt.Sprintf("%s (%s): ", name, indexeddb.OriginName(dir))
	if !drift.Drifted() {
		fmt.Fprintf(c.env.stdout, language.pick("%sok, %s\n", "%sok, %s\n"), prefix, language.count(drift.Tally.Mapped, discoveredMessageNoun))
		return
	}
	fmt.Fprintf(c.env.stdout, language.pick("%smudou: %s, %s, %s\n", "%schanged: %s, %s, %s\n"), prefix,
		language.count(len(drift.Missing), missingPathNoun), language.count(len(drift.Changed), changedPathNoun), language.count(drift.Tally.Mapped, discoveredMessageNoun))
	for _, shape := range append(drift.Missing, drift.Changed...) {
		fmt.Fprintf(c.env.stdout, "  %s %s\n", shape.Container, shape.Path)
	}
	if drift.Format != nil {
		fmt.Fprintf(c.env.stdout, "  %v\n", drift.Format)
	}
}

// regenerate replaces the schema when the comparison accepts the new one,
// and otherwise saves it as a candidate for review.
func (c *schemaChecker) regenerate(ctx context.Context, current idbmap.Schema, records []indexeddb.Record, drift idbmap.Drift) (bool, error) {
	generator, err := c.loadedGenerator()
	if err != nil {
		return false, err
	}
	found, err := idbdiscovery.NewDiscoverer(generator, idbdiscovery.DefaultCatalogLimits).Regenerate(ctx, records, current, drift)
	if err != nil && !errors.Is(err, idbdiscovery.ErrNoMessages) {
		return false, err
	}
	comparison, err := idbmap.CompareSchemas(current, found.Schema, records)
	if err != nil {
		return false, err
	}
	if !comparison.Accepted() && c.rekey && identityOnly(comparison) {
		return true, c.rekeyAndReplace(ctx, current, found.Schema, records)
	}
	if !comparison.Accepted() {
		return false, c.keepCandidate(found.Schema, comparison)
	}
	saved, err := c.dir.Replace(found.Schema)
	if err != nil {
		return false, err
	}
	fmt.Fprintf(c.env.stdout, c.env.language.pick("  substituído pela revisão %d: %s, %.0f%% dos UIDs mantidos\n", "  replaced by revision %d: %s, %.0f%% of UIDs kept\n"),
		saved.Revision, c.env.language.count(comparison.NextEvents, discoveredMessageNoun), comparison.Overlap()*percentScale)
	return true, nil
}

// identityOnly reports a replacement refused only for changing UIDs: it
// maps at least as much, so a rekey resolves the refusal.
func identityOnly(comparison idbmap.Comparison) bool {
	return comparison.NextEvents > 0 && comparison.NextEvents >= comparison.CurrentEvents
}

// rekeyAndReplace gives the indexed messages the UIDs next assigns them,
// then replaces the schema; the store copies the database first.
func (c *schemaChecker) rekeyAndReplace(ctx context.Context, current, next idbmap.Schema, records []indexeddb.Record) error {
	changes, err := idbmap.UIDChanges(current, next, records)
	if err != nil {
		return err
	}
	var report storage.RekeyReport
	err = c.env.withStore(ctx, func(_ config.Config, store storage.EventStore) error {
		report, err = store.RekeyEvents(ctx, changes)
		return err
	})
	if err != nil {
		return err
	}
	saved, err := c.dir.Replace(next)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.env.stdout, c.env.language.pick("  substituído pela revisão %d com UIDs novos: %d eventos, %d conflitos (cópia do banco em %s)\n", "  replaced by revision %d with new UIDs: %d events, %d conflicts (database copy at %s)\n"),
		saved.Revision, report.Rekeyed, report.Conflicts, report.BackupPath)
	return nil
}

func (c *schemaChecker) keepCandidate(candidate idbmap.Schema, comparison idbmap.Comparison) error {
	path, err := c.dir.SaveCandidate(candidate)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.env.stdout, c.env.language.pick("  não substituído (%s, %.0f%% dos UIDs mantidos); candidato para revisão em %s\n", "  not replaced (%s, %.0f%% of UIDs kept); candidate for review at %s\n"),
		c.env.language.count(comparison.NextEvents, discoveredMessageNoun), comparison.Overlap()*percentScale, path)
	return nil
}

func (c *schemaChecker) loadedGenerator() (ClosableGenerator, error) {
	if c.generator != nil {
		return c.generator, nil
	}
	fmt.Fprintln(c.env.stderr, c.env.language.pick("carregando o modelo local…", "loading the local model…"))
	generator, err := c.env.toolkit.LoadGenerator(c.cfg.Generation, c.env.logger)
	c.generator = generator
	return generator, err
}

func (c *schemaChecker) close() {
	if c.generator != nil {
		c.generator.Close()
	}
}
