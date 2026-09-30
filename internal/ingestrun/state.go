// Package ingestrun records what the current or last `cade ingest` is
// doing, so another terminal can follow it (`ingest status`), pause or
// stop it (`pause`, `stop`) and continue it (`resume`), and keeps
// two ingestions from running at once (issue #41).
package ingestrun

import "time"

// Status is where a run is in its life.
type Status string

const (
	StatusRunning Status = "running"
	// StatusPaused is a run whose process is frozen (SIGSTOP): it keeps
	// its models in memory and the lock, and continues where it was.
	StatusPaused      Status = "paused"
	StatusFinished    Status = "finished"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
)

// Stage is what a running ingestion is busy with.
type Stage string

const (
	StageStarting           Stage = "starting"
	StageLoadingVisionModel Stage = "loading_vision_model"
	StageDescribingImages   Stage = "describing_images"
	StageLoadingEmbedder    Stage = "loading_embedding_model"
	StageIngesting          Stage = "ingesting"
)

// Mode is how a run shares the machine.
type Mode struct {
	// Background runs were detached from the terminal that started them.
	Background bool `json:"background"`
	// Gentle runs use the ingest.background limits and the lowest priority.
	Gentle bool `json:"gentle"`
}

// State is one run, as the state file keeps it.
type State struct {
	PID int `json:"pid"`
	// Args are the source and targets, as `cade ingest` received them.
	Args       []string  `json:"args"`
	Mode       Mode      `json:"mode"`
	Status     Status    `json:"status"`
	Stage      Stage     `json:"stage"`
	StartedAt  time.Time `json:"started_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	FinishedAt time.Time `json:"finished_at"`
	// Job is the source and target being ingested, JobNumber of JobCount.
	Job       string `json:"job"`
	JobNumber int    `json:"job_number"`
	JobCount  int    `json:"job_count"`
	// Progress is the last progress line, as printed, with its ETA.
	Progress string `json:"progress"`
	Error    string `json:"error"`
	LogPath  string `json:"log_path"`
}

// IsLive reports a run not concluded yet: running or paused, with its
// process alive. One marked so whose process is gone was killed or crashed.
func (s State) IsLive(alive func(pid int) bool) bool {
	return (s.Status == StatusRunning || s.Status == StatusPaused) && alive(s.PID)
}

// IsPaused reports a live run whose process is frozen.
func (s State) IsPaused(alive func(pid int) bool) bool {
	return s.Status == StatusPaused && alive(s.PID)
}

// IsResumable reports a run that can go on: paused, or stopped before
// finishing (interrupted, failed, or gone without saying).
func (s State) IsResumable(alive func(pid int) bool) bool {
	if s.Status == StatusInterrupted || s.Status == StatusFailed {
		return true
	}
	return s.IsPaused(alive) || !s.IsLive(alive) && (s.Status == StatusRunning || s.Status == StatusPaused)
}
