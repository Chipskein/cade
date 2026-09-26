package browsersource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chipskein/cade/internal/filecopy"
)

// snapshotHistory copies the history database (plus its WAL, which holds
// the most recent visits) into a temporary directory. Browsers keep the
// live file locked, and reading a copy guarantees we never modify it.
// The caller must remove the returned directory.
func snapshotHistory(historyPath string) (string, string, error) {
	tempDir, err := os.MkdirTemp("", "cade-browser-*")
	if err != nil {
		return "", "", fmt.Errorf("create snapshot directory: %w", err)
	}
	snapshot := filepath.Join(tempDir, "history.sqlite")
	if err := filecopy.File(historyPath, snapshot); err != nil {
		os.RemoveAll(tempDir)
		return "", "", fmt.Errorf("snapshot browser history: %w", err)
	}
	if err := copyOptionalFile(historyPath+"-wal", snapshot+"-wal"); err != nil {
		os.RemoveAll(tempDir)
		return "", "", err
	}
	return tempDir, snapshot, nil
}

func copyOptionalFile(source, destination string) error {
	err := filecopy.File(source, destination)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
