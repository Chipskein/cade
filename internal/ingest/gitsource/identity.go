package gitsource

import (
	"context"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/event"
)

// AutoIdentity in sources.git_identities stands for the repository's
// configured user: `git config user.email` and `user.name`, which git
// reads from the repository and then the global configuration.
const AutoIdentity = "auto"

// resolveIdentities expands "auto" and lowercases every identity. A
// repository without a configured user resolves "auto" to nothing, which
// leaves authorship unknown rather than calling every commit someone
// else's.
func (c *Collector) resolveIdentities(ctx context.Context, repository string) []string {
	var identities []string
	for _, identity := range c.identities {
		if identity != AutoIdentity {
			identities = append(identities, strings.ToLower(strings.TrimSpace(identity)))
			continue
		}
		identities = append(identities, c.configuredUser(ctx, repository)...)
	}
	return identities
}

func (c *Collector) configuredUser(ctx context.Context, repository string) []string {
	var user []string
	for _, key := range []string{"user.email", "user.name"} {
		output, err := c.runner.Run(ctx, repository, "git", "config", key)
		if value := strings.ToLower(strings.TrimSpace(string(output))); err == nil && value != "" {
			user = append(user, value)
		}
	}
	return user
}

// authorshipOf matches the commit's author email or name against the
// user's identities, ignoring case.
func authorshipOf(name, email string, identities []string) event.Authorship {
	if len(identities) == 0 {
		return event.AuthorshipUnknown
	}
	if slices.Contains(identities, strings.ToLower(email)) || slices.Contains(identities, strings.ToLower(name)) {
		return event.AuthorshipMine
	}
	return event.AuthorshipOther
}

// CommitAuthorship names the repository and the identities of the last
// collection, so ingestion can mark commits stored before them.
func (c *Collector) CommitAuthorship() (string, []string) {
	return c.resolvedRepository, c.resolved
}
