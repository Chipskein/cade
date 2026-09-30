package ingestrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// File names inside the state directory.
const (
	stateFileName = "ingest-state.json"
	lockFileName  = "ingest.lock"
	logFileName   = "ingest.log"
)

// Owner-only, like the database: the state and the log name the targets.
const (
	stateDirMode  = 0o700
	stateFileMode = 0o600
)

// Dir holds the state file, the lock and the background log.
type Dir string

// DefaultDir is $XDG_STATE_HOME/cade, or ~/.local/state/cade.
//
//	dir, err := ingestrun.DefaultDir(os.Getenv, os.UserHomeDir)
func DefaultDir(getenv func(string) string, homeDir func() (string, error)) (Dir, error) {
	if state := getenv("XDG_STATE_HOME"); filepath.IsAbs(state) {
		return Dir(filepath.Join(state, "cade")), nil
	}
	home, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory for the ingest state: %w", err)
	}
	return Dir(filepath.Join(home, ".local", "state", "cade")), nil
}

func (d Dir) StatePath() string { return filepath.Join(string(d), stateFileName) }
func (d Dir) LockPath() string  { return filepath.Join(string(d), lockFileName) }
func (d Dir) LogPath() string   { return filepath.Join(string(d), logFileName) }

// Create makes the directory, owner-only.
func (d Dir) Create() error {
	if err := os.MkdirAll(string(d), stateDirMode); err != nil {
		return fmt.Errorf("create ingest state directory %q: %w", d, err)
	}
	return nil
}

// StateFile keeps the state of the current or last run.
type StateFile interface {
	// Read returns false when no run was ever recorded.
	Read() (State, bool, error)
	Write(state State) error
}

// JSONStateFile is a StateFile on disk.
type JSONStateFile struct {
	Dir Dir
}

func (f JSONStateFile) Read() (State, bool, error) {
	raw, err := os.ReadFile(f.Dir.StatePath())
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("read ingest state %q: %w", f.Dir.StatePath(), err)
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, false, fmt.Errorf("parse ingest state %q, expected the JSON object cade writes: %w", f.Dir.StatePath(), err)
	}
	return state, true, nil
}

// Write replaces the file through a rename, so a reader never sees half.
func (f JSONStateFile) Write(state State) error {
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ingest state: %w", err)
	}
	if err := f.Dir.Create(); err != nil {
		return err
	}
	temporary := f.Dir.StatePath() + ".tmp"
	if err := os.WriteFile(temporary, encoded, stateFileMode); err != nil {
		return fmt.Errorf("write ingest state %q: %w", temporary, err)
	}
	if err := os.Rename(temporary, f.Dir.StatePath()); err != nil {
		return fmt.Errorf("replace ingest state %q: %w", f.Dir.StatePath(), err)
	}
	return nil
}
