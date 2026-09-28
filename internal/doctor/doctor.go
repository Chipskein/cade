// Package doctor checks an installation for `cade doctor`: the config
// file, the models, SQLite's keyword search, the database and every
// configured source path. It reports problems as codes; the CLI words
// them in the user's language and says how to fix each one.
package doctor

import (
	"io/fs"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
)

// Subject is what a finding is about.
type Subject int

const (
	SubjectConfigFile Subject = iota
	SubjectEmbeddingModel
	SubjectGenerationModel
	SubjectVisionProjector
	SubjectKeywordSearch
	SubjectDatabase
	SubjectSource
)

// Severity orders findings: warnings leave cade usable, failures do not.
type Severity int

const (
	SeverityOK Severity = iota
	SeverityWarning
	SeverityFailure
)

// Finding is one checked item and what, if anything, is wrong with it.
type Finding struct {
	Subject Subject
	// Source is the ingestion source of a SubjectSource finding ("git").
	Source string
	// Setting is the config key that sets Path ("generation.model_path").
	Setting string
	Path    string
	Problem Problem
	// Database and ConfiguredModel detail a SubjectDatabase finding.
	Database        storage.DatabaseState
	ConfiguredModel string
}

// Severity is the finding's problem's severity.
func (f Finding) Severity() Severity {
	return f.Problem.Severity()
}

// Diagnose checks everything cfg points at. fsys is rooted at "/" (see
// rootfs); database comes from reading the database without opening it
// as a store, so diagnosing never migrates it.
//
//	findings := doctor.Diagnose(os.DirFS("/"), configPath, cfg, state)
func Diagnose(fsys fs.FS, configPath string, cfg config.Config, database storage.DatabaseState) []Finding {
	findings := []Finding{
		checkConfigFile(fsys, configPath),
		checkModel(fsys, SubjectEmbeddingModel, "embedding.model_path", cfg.Embedding.ModelPath),
		checkModel(fsys, SubjectGenerationModel, "generation.model_path", cfg.Generation.ModelPath),
		checkKeywordSearch(database),
		checkDatabase(cfg, database),
	}
	if gates := checkThresholdCalibration(cfg, database); gates.Problem != ProblemNone {
		findings = append(findings, gates)
	}
	// Only `ingest` with images on loads the projector; without them a
	// missing file is not a problem.
	if cfg.Sources.Images {
		findings = append(findings, checkModel(fsys, SubjectVisionProjector, "vision.projector_path", cfg.Vision.ProjectorPath))
	}
	return append(findings, checkSources(fsys, cfg.Sources)...)
}

// ConfiguredCalibration is the gates cfg searches with, for its embedding
// model.
func ConfiguredCalibration(cfg config.Config) storage.ThresholdCalibration {
	return storage.ThresholdCalibration{Model: cfg.Embedding.ModelName(), MaxDistance: cfg.Retrieval.MaxDistance,
		MaxBestDistance: cfg.Retrieval.MaxBestDistance}
}

// checkThresholdCalibration flags gates set for another embedding model;
// the finding's Database carries the model they were set for.
func checkThresholdCalibration(cfg config.Config, database storage.DatabaseState) Finding {
	current := ConfiguredCalibration(cfg)
	calibration := database.ThresholdCalibration.OrIndexedWith(database.EmbeddingModel, current)
	if !database.Exists || !calibration.OutdatedFor(current) {
		return Finding{}
	}
	database.ThresholdCalibration = calibration
	return Finding{Subject: SubjectDatabase, Setting: thresholdSettings, Path: cfg.DatabasePath,
		Problem: ProblemThresholdModelMismatch, Database: database, ConfiguredModel: current.Model}
}

const thresholdSettings = "retrieval.max_distance, retrieval.max_best_distance"

// CountBySeverity counts the findings of severity.
func CountBySeverity(findings []Finding, severity Severity) int {
	count := 0
	for _, finding := range findings {
		if finding.Severity() == severity {
			count++
		}
	}
	return count
}
