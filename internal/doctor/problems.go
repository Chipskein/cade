package doctor

// Problem is what is wrong with a checked item; ProblemNone means nothing.
type Problem int

const (
	ProblemNone Problem = iota
	// Warnings: cade works, but the user likely wants to act.
	ProblemNoConfigFile
	ProblemNoSources
	ProblemNoDatabase
	ProblemMigrationPending
	ProblemThresholdModelMismatch
	// Failures: a command will fail or return wrong results.
	ProblemMissing
	ProblemNotAFile
	ProblemNotADirectory
	ProblemNotGGUF
	ProblemNotSQLite
	ProblemNotGitRepository
	ProblemNotLevelDB
	ProblemNoFTS5
	ProblemSchemaTooNew
	ProblemEmbeddingMismatch
	ProblemReindexPending
)

// Severity classifies the problem.
func (p Problem) Severity() Severity {
	switch {
	case p == ProblemNone:
		return SeverityOK
	case p < ProblemMissing:
		return SeverityWarning
	default:
		return SeverityFailure
	}
}
