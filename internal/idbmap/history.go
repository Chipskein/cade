package idbmap

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// historyDir keeps every replaced schema beside the current ones, named
// <name>.<revision>.json, so a regeneration can always be undone.
const historyDir = "history"

// ErrNoEarlierRevision is returned by Rollback when history holds nothing
// older than the current schema.
var ErrNoEarlierRevision = errors.New("no earlier revision in history")

// HistoryPath is where revision of the schema called name is kept.
func (d SchemaDir) HistoryPath(name string, revision int) string {
	return filepath.Join(d.path, historyDir, name+"."+strconv.Itoa(revision)+schemaFileSuffix)
}

// CandidatePath is where a regenerated schema waits for review.
func (d SchemaDir) CandidatePath(name string) string {
	return filepath.Join(d.path, name+candidateSuffix)
}

// Replace saves next as its name's schema, keeping the current one in
// history; next is numbered above every revision kept so far.
//
//	saved, err := dir.Replace(regenerated)
func (d SchemaDir) Replace(next Schema) (Schema, error) {
	current, err := d.Load(next.Name)
	if err != nil {
		return Schema{}, err
	}
	revisions, err := d.historyRevisions(next.Name)
	if err != nil {
		return Schema{}, err
	}
	next.Revision = max(current.Revision, maxRevision(revisions)) + 1
	if err := d.archive(current); err != nil {
		return Schema{}, err
	}
	return next, d.Save(next, true)
}

// Rollback restores the newest revision older than the current schema,
// keeping the current one in history too.
func (d SchemaDir) Rollback(name string) (Schema, error) {
	current, err := d.Load(name)
	if err != nil {
		return Schema{}, err
	}
	previous, err := d.previousRevision(name, current.Revision)
	if err != nil {
		return Schema{}, err
	}
	if err := d.archive(current); err != nil {
		return Schema{}, err
	}
	return previous, d.Save(previous, true)
}

// SaveCandidate writes a schema that was not accepted as a replacement,
// for the user to review; ingest never reads it.
func (d SchemaDir) SaveCandidate(schema Schema) (string, error) {
	encoded, err := EncodeSchema(schema)
	if err != nil {
		return "", err
	}
	path := d.CandidatePath(schema.Name)
	return path, d.files.WriteFile(path, encoded)
}

func (d SchemaDir) archive(schema Schema) error {
	encoded, err := EncodeSchema(schema)
	if err != nil {
		return err
	}
	return d.files.WriteFile(d.HistoryPath(schema.Name, schema.Revision), encoded)
}

func (d SchemaDir) previousRevision(name string, current int) (Schema, error) {
	revisions, err := d.historyRevisions(name)
	if err != nil {
		return Schema{}, err
	}
	best := -1
	for _, revision := range revisions {
		if revision < current && revision > best {
			best = revision
		}
	}
	if best < 0 {
		return Schema{}, fmt.Errorf("%w: schema %q at revision %d", ErrNoEarlierRevision, name, current)
	}
	return d.readFile(d.HistoryPath(name, best))
}

// historyRevisions lists the revisions kept for name.
func (d SchemaDir) historyRevisions(name string) ([]int, error) {
	files, err := d.files.ListFiles(filepath.Join(d.path, historyDir))
	if err != nil {
		return nil, err
	}
	var revisions []int
	for _, file := range files {
		number, found := strings.CutPrefix(strings.TrimSuffix(file, schemaFileSuffix), name+".")
		if revision, err := strconv.Atoi(number); found && err == nil {
			revisions = append(revisions, revision)
		}
	}
	return revisions, nil
}

func maxRevision(revisions []int) int {
	highest := 0
	for _, revision := range revisions {
		highest = max(highest, revision)
	}
	return highest
}
