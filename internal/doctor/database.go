package doctor

import (
	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/storage"
)

func checkKeywordSearch(database storage.DatabaseState) Finding {
	finding := Finding{Subject: SubjectKeywordSearch}
	if !database.FTS5 {
		finding.Problem = ProblemNoFTS5
	}
	return finding
}

func checkDatabase(cfg config.Config, database storage.DatabaseState) Finding {
	configured := cfg.Embedding.ModelName()
	return Finding{
		Subject: SubjectDatabase, Setting: "database_path", Path: cfg.DatabasePath,
		Problem: databaseProblem(database, configured), Database: database, ConfiguredModel: configured,
	}
}

// databaseProblem reports the most pressing issue: one that breaks
// commands before one that the next command fixes by itself.
func databaseProblem(database storage.DatabaseState, configuredModel string) Problem {
	switch {
	case !database.Exists:
		return ProblemNoDatabase
	case database.SchemaVersion > database.LatestSchemaVersion:
		return ProblemSchemaTooNew
	case database.EmbeddingModel != "" && database.EmbeddingModel != configuredModel:
		return ProblemEmbeddingMismatch
	case database.ReindexPending:
		return ProblemReindexPending
	case database.SchemaVersion < database.LatestSchemaVersion:
		return ProblemMigrationPending
	}
	return ProblemNone
}
