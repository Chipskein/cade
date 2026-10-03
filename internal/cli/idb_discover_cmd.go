package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/idbdiscovery"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/webstore"
)

var discoveredMessageNoun = nounForms{"mensagem", "mensagens", "message", "messages"}

// idbDiscoverRequest is what the command line asked for.
type idbDiscoverRequest struct {
	dir, name, source          string
	force, printOnly, rollback bool
}

// runIDBDiscover asks the local model for a schema of an application's
// IndexedDB and saves it, so `ingest <name>` can read the application
// without code of its own.
func runIDBDiscover(ctx context.Context, env commandEnv, args []string) error {
	request, err := parseIDBDiscover(env, args)
	if err != nil {
		return err
	}
	cfg, err := env.loadConfig()
	if err != nil {
		return err
	}
	if request.rollback {
		return env.rollbackSchema(cfg, request.name)
	}
	found, err := env.discoverSchema(ctx, cfg, request)
	if errors.Is(err, idbdiscovery.ErrNoMessages) || request.printOnly {
		return errors.Join(printSchema(env, found.Schema), err)
	}
	if err != nil {
		return err
	}
	return env.saveDiscovered(cfg, request, found)
}

func parseIDBDiscover(env commandEnv, args []string) (idbDiscoverRequest, error) {
	flags := newFlagSet("idb-discover", env.stderr, env.language)
	name := flags.String("name", "", env.language.pick("nome do schema e da fonte (ex.: whatsapp)", "schema and source name (e.g. whatsapp)"))
	source := flags.String("source", "", env.language.pick("fonte dos eventos, se diferente do nome", "event source, if not the name"))
	force := flags.Bool("force", false, env.language.pick("substitui um schema já salvo", "replaces a saved schema"))
	rollback := flags.Bool("rollback", false, env.language.pick("volta o schema --name à revisão anterior", "brings schema --name back to its previous revision"))
	printOnly := flags.Bool("print", false, env.language.pick("só mostra o schema, sem salvar", "only prints the schema, without saving"))
	positional, err := parseCommandFlags(flags, args)
	if err != nil {
		return idbDiscoverRequest{}, err
	}
	if *rollback && *name != "" && len(positional) == 0 {
		return idbDiscoverRequest{name: *name, rollback: true}, nil
	}
	if len(positional) != 1 || *name == "" {
		return idbDiscoverRequest{}, fmt.Errorf("%s: cade idb-discover --name whatsapp ~/.floorp/<perfil>/storage/default/https+++web.whatsapp.com/idb",
			env.language.pick("informe --name e um diretório", "give --name and one directory"))
	}
	return idbDiscoverRequest{dir: positional[0], name: *name, source: cmp.Or(*source, *name), force: *force, printOnly: *printOnly}, nil
}

func (env commandEnv) discoverSchema(ctx context.Context, cfg config.Config, request idbDiscoverRequest) (idbdiscovery.Discovery, error) {
	records, err := env.readDetectedStore(config.ExpandHome(request.dir))
	if err != nil {
		return idbdiscovery.Discovery{}, err
	}
	fmt.Fprintln(env.stderr, env.language.pick("carregando o modelo local…", "loading the local model…"))
	generator, err := env.toolkit.LoadGenerator(cfg.Generation, env.logger)
	if err != nil {
		return idbdiscovery.Discovery{}, err
	}
	defer generator.Close()
	discoverer := idbdiscovery.NewDiscoverer(generator, idbdiscovery.DefaultCatalogLimits)
	return discoverer.Discover(ctx, records, request.name, request.source)
}

// readDetectedStore reads location with the reader of its layout, so a
// schema can be discovered for any storage cade reads.
func (env commandEnv) readDetectedStore(location string) ([]webstore.Record, error) {
	reader, err := env.toolkit.StoreReaders.Detect(location)
	if err != nil {
		return nil, err
	}
	return reader.Read(location)
}

func printSchema(env commandEnv, schema idbmap.Schema) error {
	encoded, err := idbmap.EncodeSchema(schema)
	if err != nil {
		return err
	}
	_, err = env.stdout.Write(encoded)
	return err
}

// rollbackSchema undoes the last replacement of a schema, by hand or by
// `idb-check --update`.
func (env commandEnv) rollbackSchema(cfg config.Config, name string) error {
	restored, err := idbmap.NewSchemaDir(env.toolkit.SchemaFiles, cfg.Sources.IndexedDBSchemaDir).Rollback(name)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.stdout, env.language.pick("schema %q voltou à revisão %d.\n", "schema %q is back at revision %d.\n"), name, restored.Revision)
	return nil
}

// saveDiscovered writes the schema and says how to point ingest at it.
func (env commandEnv) saveDiscovered(cfg config.Config, request idbDiscoverRequest, found idbdiscovery.Discovery) error {
	dir := idbmap.NewSchemaDir(env.toolkit.SchemaFiles, cfg.Sources.IndexedDBSchemaDir)
	if err := dir.Save(found.Schema, request.force); err != nil {
		if errors.Is(err, idbmap.ErrSchemaExists) {
			return fmt.Errorf("%w (%s)", err, env.language.pick("use --force para substituir, ou --print para só ver", "use --force to replace it, or --print to only see it"))
		}
		return err
	}
	fmt.Fprintf(env.stdout, env.language.pick("schema %q salvo em %s: %s de %d registros do store %q.\n", "schema %q saved to %s: %s from %d records of store %q.\n"),
		found.Schema.Name, dir.FilePath(found.Schema.Name), env.language.count(found.Events, discoveredMessageNoun), found.Tally.StoreRecords, found.Schema.Records.Container)
	hint, _ := json.Marshal(map[string][]string{found.Schema.Name: {request.dir}})
	fmt.Fprintf(env.stdout, env.language.pick("Para ingerir, acrescente em sources.indexeddb_dirs da configuração: %s\ne rode: cade ingest %s\n", "To ingest, add to sources.indexeddb_dirs in the config: %s\nand run: cade ingest %s\n"),
		hint, found.Schema.Name)
	return nil
}
