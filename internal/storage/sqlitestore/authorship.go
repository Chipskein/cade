package sqlitestore

import (
	"context"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/event"
)

// MarkCommitAuthorship marks every stored commit of repository as the
// user's or someone else's by its author email or name. It covers commits
// ingested before identities were known, which ingestion skips as already
// stored; only rows whose mark changes are written. Returns how many.
func (s *Store) MarkCommitAuthorship(ctx context.Context, repository string, identities []string) (int, error) {
	if len(identities) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(identities)), ", ")
	authorship := fmt.Sprintf(`CASE WHEN lower(json_extract(metadata, '$.email')) IN (%[1]s)
		OR lower(json_extract(metadata, '$.author')) IN (%[1]s) THEN '%[2]s' ELSE '%[3]s' END`,
		placeholders, event.AuthorshipMine, event.AuthorshipOther)
	query := `UPDATE events SET metadata = json_set(metadata, '$.authorship', ` + authorship + `)
		WHERE source = ? AND json_extract(metadata, '$.repository') = ?
		AND coalesce(json_extract(metadata, '$.authorship'), '') != ` + authorship
	args := append(identityArgs(identities), string(event.SourceGit), repository)
	result, err := s.db.ExecContext(ctx, query, append(args, identityArgs(identities)...)...)
	if err != nil {
		return 0, fmt.Errorf("mark authorship of commits in %q: %w", repository, err)
	}
	changed, err := result.RowsAffected()
	return int(changed), err
}

// identityArgs repeats the identities for the email and the name tests.
func identityArgs(identities []string) []any {
	args := make([]any, 0, 2*len(identities))
	for range 2 {
		for _, identity := range identities {
			args = append(args, strings.ToLower(identity))
		}
	}
	return args
}
