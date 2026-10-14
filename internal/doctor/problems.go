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
	ProblemVisionModelMismatch
	ProblemEmptyVectorSlots
	// ProblemPlainCopies: the database is encrypted, but copies from
	// before are not (#111).
	ProblemPlainCopies
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
	// ProblemKeyNotFound: the database is encrypted and its key was not
	// found, or did not open it (#111).
	ProblemKeyNotFound
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
