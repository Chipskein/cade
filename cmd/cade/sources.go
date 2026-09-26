package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/browsersource"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/ingest/gitsource"
	"github.com/chipskein/cade/internal/ingest/teamssource"
)

// sourceSpecs registers every ingestable source. A new source (RF1.4
// Teams) is one more entry here plus its collector package.
func sourceSpecs(cfg config.Config) []ingest.SourceSpec {
	return []ingest.SourceSpec{
		{Name: "git", DefaultTargets: cfg.Sources.GitRepositories, NewCollector: gitCollectorFactory(cfg.Sources)},
		{Name: "browser", DefaultTargets: cfg.Sources.BrowserHistories, NewCollector: newBrowserCollector},
		{Name: "file", DefaultTargets: cfg.Sources.Directories, NewCollector: fileCollectorFactory(cfg.Sources)},
		{Name: "teams", DefaultTargets: cfg.Sources.TeamsIndexedDBDirs, NewCollector: newTeamsCollector},
	}
}

func newTeamsCollector(indexedDBDir string) (ingest.EventCollector, error) {
	return teamssource.NewCollector(indexeddb.ReadDirectory, indexedDBDir), nil
}

func gitCollectorFactory(sources config.SourcesConfig) func(string) (ingest.EventCollector, error) {
	return func(repository string) (ingest.EventCollector, error) {
		return gitsource.NewCollector(gitsource.ExecRunner{}, repository, sources.GitAuthors, sources.GitIdentities), nil
	}
}

func newBrowserCollector(historyPath string) (ingest.EventCollector, error) {
	return browsersource.NewCollector(openSQLiteFile, historyPath), nil
}

func fileCollectorFactory(sources config.SourcesConfig) func(string) (ingest.EventCollector, error) {
	return func(directory string) (ingest.EventCollector, error) {
		root, err := filepath.Abs(directory)
		if err != nil {
			return nil, fmt.Errorf("resolve directory %q: %w", directory, err)
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("directory %q does not exist or is not a directory", root)
		}
		opts := filesource.Options{IgnoredDirNames: sources.IgnoredDirNames, MaxFileBytes: sources.MaxFileBytes}
		return filesource.NewCollector(os.DirFS(root), root, opts), nil
	}
}
