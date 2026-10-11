package ingestrun

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// stopRequestFileName exists while `ingest stop` waits for a run to stop.
const stopRequestFileName = "ingest.stop"

// StopRequests let `ingest stop` ask the run to stop through the state
// directory, where no signal can (Windows has no SIGTERM, #54). The run
// checks between steps and stops as on Ctrl-C, keeping what it stored.
type StopRequests interface {
	// Ask records the request.
	Ask() error
	// Asked reports whether a stop was asked for.
	Asked() bool
	// Clear removes the request, so it never stops a later run.
	Clear() error
}

// FileStopRequests keep the request as a file in Dir.
type FileStopRequests struct {
	Dir Dir
}

func (d Dir) StopRequestPath() string { return filepath.Join(string(d), stopRequestFileName) }

func (r FileStopRequests) Ask() error {
	if err := r.Dir.Create(); err != nil {
		return err
	}
	if err := os.WriteFile(r.Dir.StopRequestPath(), nil, stateFileMode); err != nil {
		return fmt.Errorf("ask the ingestion to stop through %q: %w", r.Dir.StopRequestPath(), err)
	}
	return nil
}

func (r FileStopRequests) Asked() bool {
	_, err := os.Stat(r.Dir.StopRequestPath())
	return err == nil
}

func (r FileStopRequests) Clear() error {
	err := os.Remove(r.Dir.StopRequestPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear the stop request %q: %w", r.Dir.StopRequestPath(), err)
	}
	return nil
}
