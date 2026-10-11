package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/browsersource"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/ingest/gitsource"
	"github.com/chipskein/cade/internal/ingest/idbsource"
	"github.com/chipskein/cade/internal/ingest/teamssource"
	"github.com/chipskein/cade/internal/ingest/whatsappsource"
)

// sourceSpecs registers every ingestable source. A new source (RF1.4
// Teams) is one more entry here plus its collector package; an
// application read through a saved IndexedDB schema needs neither.
func sourceSpecs(cfg config.Config, captions ingest.ImageCaptions) []ingest.SourceSpec {
	builtIn := []ingest.SourceSpec{
		{Name: "git", DefaultTargets: cfg.Sources.GitRepositories, NewCollector: gitCollectorFactory(cfg.Sources)},
		{Name: "browser", DefaultTargets: cfg.Sources.BrowserHistories, NewCollector: newBrowserCollector},
		{Name: "file", DefaultTargets: cfg.Sources.Directories, NewCollector: fileCollectorFactory(cfg.Sources, captions)},
		{Name: "teams", DefaultTargets: cfg.Sources.TeamsIndexedDBDirs, NewCollector: teamsCollectorFactory(captions)},
		{Name: whatsappBackupSource, DefaultTargets: cfg.Sources.WhatsAppBackups, NewCollector: newWhatsAppBackupCollector},
	}
	return append(builtIn, schemaSpecs(cfg.Sources, idbmap.NewSchemaDir(idbmap.OSSchemaFiles{}, cfg.Sources.IndexedDBSchemaDir), builtIn)...)
}

// schemaSpecs registers one source per saved IndexedDB schema. A schema
// named like a built-in source is left out rather than hide it, and an
// unreadable directory registers nothing: ingest then names the source as
// unknown, and `cade schema-discover` recreates the schema.
func schemaSpecs(sources config.SourcesConfig, dir idbmap.SchemaDir, builtIn []ingest.SourceSpec) []ingest.SourceSpec {
	names, err := dir.Names()
	if err != nil {
		return nil
	}
	var specs []ingest.SourceSpec
	for _, name := range names {
		if _, taken := ingest.FindSource(builtIn, name); taken == nil {
			continue
		}
		specs = append(specs, ingest.SourceSpec{Name: name, DefaultTargets: sources.IndexedDBDirs[name], NewCollector: schemaCollectorFactory(sources, dir, name)})
	}
	return specs
}

// schemaCollectorFactory loads the schema when its source is ingested, so
// a hand edit gone wrong fails there, naming the file.
func schemaCollectorFactory(sources config.SourcesConfig, dir idbmap.SchemaDir, name string) func(string) (ingest.EventCollector, error) {
	return func(location string) (ingest.EventCollector, error) {
		schema, err := dir.Load(name)
		if err != nil {
			return nil, err
		}
		readers, err := storeReaders(sources)
		if err != nil {
			return nil, err
		}
		return idbsource.NewCollector(readers, location, schema)
	}
}

// whatsappBackupSource is not "whatsapp": that name is the WhatsApp Web
// schema's, which a built-in source would hide; both store their events
// under the source whatsapp (#77).
const whatsappBackupSource = "whatsapp-backup"

func newWhatsAppBackupCollector(path string) (ingest.EventCollector, error) {
	return whatsappsource.NewCollector(openSQLiteFile, path), nil
}

func teamsCollectorFactory(captions ingest.ImageCaptions) func(string) (ingest.EventCollector, error) {
	return func(indexedDBDir string) (ingest.EventCollector, error) {
		return teamssource.NewCollector(readIndexedDB, indexedDBDir).WithCaptions(captions), nil
	}
}

// teamsPictures reads the pictures of a Teams IndexedDB's messages from
// the HTTP cache of the same browser profile.
func teamsPictures(ctx context.Context, indexedDBDir string) (map[string][]byte, error) {
	configDir, _ := os.UserConfigDir()
	cacheDir, _ := os.UserCacheDir()
	collector := teamssource.NewCollector(readIndexedDB, indexedDBDir)
	return collector.Pictures(ctx, teamssource.HTTPCacheDirs(indexedDBDir, configDir, cacheDir))
}

func gitCollectorFactory(sources config.SourcesConfig) func(string) (ingest.EventCollector, error) {
	return func(repository string) (ingest.EventCollector, error) {
		return gitsource.NewCollector(gitsource.ExecRunner{}, repository, sources.GitAuthors, sources.GitIdentities), nil
	}
}

func newBrowserCollector(historyPath string) (ingest.EventCollector, error) {
	return browsersource.NewCollector(openSQLiteFile, historyPath), nil
}

func fileCollectorFactory(sources config.SourcesConfig, captions ingest.ImageCaptions) func(string) (ingest.EventCollector, error) {
	return func(directory string) (ingest.EventCollector, error) {
		root, err := filepath.Abs(directory)
		if err != nil {
			return nil, fmt.Errorf("resolve directory %q: %w", directory, err)
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("directory %q does not exist or is not a directory", root)
		}
		return filesource.NewCollector(os.DirFS(root), root, filesource.OptionsFor(sources, captions)), nil
	}
}
