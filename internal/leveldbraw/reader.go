package leveldbraw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadLatest returns the newest live value of every key found in the
// directory's tables (.ldb/.sst) and journals (.log). The directory must not
// be written to concurrently; callers pass a snapshot copy.
//
//	entries, err := leveldbraw.ReadLatest("/tmp/snapshot/https_x_0.indexeddb.leveldb")
func ReadLatest(dir string) ([]Entry, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list LevelDB directory %q: %w", dir, err)
	}
	versions := newVersionSet()
	for _, file := range files {
		if err := readFile(filepath.Join(dir, file.Name()), versions); err != nil {
			return nil, err
		}
	}
	return versions.liveEntries(), nil
}

func readFile(path string, versions *versionSet) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ldb", ".sst":
		return readTableFile(path, versions)
	case ".log":
		return readJournalFile(path, versions)
	}
	return nil
}

func readTableFile(path string, versions *versionSet) error {
	table, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read table %q: %w", path, err)
	}
	err = tableEntries(table, func(internalKey, value []byte) error {
		key, sequence, deleted, err := parseInternalKey(internalKey)
		if err == nil {
			versions.record(key, sequence, deleted, value)
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("table %q: %w", path, err)
	}
	return nil
}

func readJournalFile(path string, versions *versionSet) error {
	journal, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read journal %q: %w", path, err)
	}
	for _, batch := range journalBatches(journal) {
		if err := decodeBatch(batch, versions); err != nil {
			return fmt.Errorf("journal %q: %w", path, err)
		}
	}
	return nil
}
