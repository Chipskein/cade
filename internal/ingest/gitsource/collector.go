// Package gitsource ingests commits from a Git repository (RF1.1).
package gitsource

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
)

// Record (\x1e) and field (\x1f) separators cannot appear in commit metadata,
// so parsing never has to guess where a multi-line message ends.
const (
	recordSeparator = "\x1e"
	fieldSeparator  = "\x1f"
	logFormat       = "--pretty=format:%x1e%H%x1f%aI%x1f%an%x1f%ae%x1f%B%x1f"
	commitFieldSize = 6
)

// Collector reads the commit history of one repository.
type Collector struct {
	runner     CommandRunner
	repository string
	authors    []string
	// identities are the user's emails or names, or AutoIdentity; each
	// commit is marked as the user's or someone else's against them.
	identities         []string
	resolved           []string
	resolvedRepository string
}

// NewCollector reads repository with runner, keeping only commits whose
// author matches one of authors (all commits when empty), and marking each
// as the user's when its author is one of identities.
//
//	collector := gitsource.NewCollector(gitsource.ExecRunner{}, "~/src/app", nil, []string{gitsource.AutoIdentity})
func NewCollector(runner CommandRunner, repository string, authors, identities []string) *Collector {
	return &Collector{runner: runner, repository: repository, authors: authors, identities: identities}
}

// CollectEvents emits one event per commit reachable from any branch, tag or
// remote, so work on unmerged branches is not lost (RNF3.1).
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	repository, err := filepath.Abs(c.repository)
	if err != nil {
		return fmt.Errorf("resolve repository path %q: %w", c.repository, err)
	}
	c.resolvedRepository, c.resolved = repository, c.resolveIdentities(ctx, repository)
	output, err := c.runner.Run(ctx, repository, "git", c.logArguments()...)
	if err != nil {
		return fmt.Errorf("read git history of %q: %w", repository, err)
	}
	return emitCommits(string(output), repository, c.resolved, emit)
}

func (c *Collector) logArguments() []string {
	args := []string{"-c", "core.quotepath=off", "log", "--branches", "--tags", "--remotes", "HEAD",
		"--no-color", "--name-only", logFormat}
	for _, author := range c.authors {
		args = append(args, "--author="+author)
	}
	return args
}

func emitCommits(output, repository string, identities []string, emit ingest.EmitFunc) error {
	for _, record := range strings.Split(output, recordSeparator)[1:] {
		ev, err := parseCommit(record, repository, identities)
		if err != nil {
			return err
		}
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}

func parseCommit(record, repository string, identities []string) (event.Event, error) {
	fields := strings.SplitN(record, fieldSeparator, commitFieldSize)
	if len(fields) != commitFieldSize {
		return event.Event{}, fmt.Errorf("parse git log record %q: expected %d fields, got %d", record, commitFieldSize, len(fields))
	}
	hash, rawDate, author, email, message := fields[0], fields[1], fields[2], fields[3], strings.TrimSpace(fields[4])
	authoredAt, err := time.Parse(time.RFC3339, rawDate)
	if err != nil {
		return event.Event{}, fmt.Errorf("parse date %q of commit %s, expected RFC 3339: %w", rawDate, hash, err)
	}
	files := changedFiles(fields[5])
	return event.Event{
		UID:       event.StableID(event.SourceGit, hash),
		Timestamp: authoredAt,
		Source:    event.SourceGit,
		Content:   commitContent(message, files),
		Metadata: event.Commit{Repository: repository, Hash: hash, Author: author, Email: email, Files: files,
			Authorship: authorshipOf(author, email, identities)}.Metadata(),
	}, nil
}

func changedFiles(block string) []string {
	var files []string
	for _, line := range strings.Split(block, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

func commitContent(message string, files []string) string {
	if len(files) == 0 {
		return message
	}
	return message + "\n\nArquivos alterados: " + strings.Join(files, ", ")
}
