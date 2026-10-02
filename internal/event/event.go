// Package event defines the normalized activity record every source is
// converted into (RF2). Queries only ever see this type, which is what lets a
// new source become searchable without touching timeline or RAG code (RNF4).
package event

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Source identifies where an event came from.
type Source string

const (
	SourceGit     Source = "git"
	SourceBrowser Source = "browser"
	SourceFile    Source = "file"
	SourceTeams   Source = "teams"
)

// Metadata holds source-specific attributes (commit hash, URL, file path...).
type Metadata map[string]string

// Event is one unit of user activity.
type Event struct {
	// UID is the deduplication key; see StableID.
	UID       string
	Timestamp time.Time
	Source    Source
	Content   string
	Metadata  Metadata
}

// StableID derives a deterministic deduplication key from the source and the
// fields that make an event unique within it. Parts are NUL-separated so that
// ("ab","c") and ("a","bc") never collide (RNF3.2).
//
//	id := event.StableID(event.SourceGit, commitHash)
func StableID(source Source, parts ...string) string {
	digest := sha256.New()
	digest.Write([]byte(source))
	for _, part := range parts {
		digest.Write([]byte{0})
		digest.Write([]byte(part))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// RevisionKey is the optional metadata entry ordering versions of the same
// event (a Teams message's edit stamp): ingestion only replaces a stored
// event with a newer revision.
const RevisionKey = "revision"

// Revision returns the event's revision, if it has a numeric one.
func (e Event) Revision() (int64, bool) {
	revision, err := strconv.ParseInt(e.Metadata[RevisionKey], 10, 64)
	return revision, err == nil
}

// SchemaKey and SchemaRevisionKey record which IndexedDB schema, at which
// revision, mapped an event. A regenerated schema that reads the same
// message better replaces it on the next ingest, even at the same message
// revision (see ingest's replaces).
const (
	SchemaKey         = "schema"
	SchemaRevisionKey = "schema_revision"
)

// SchemaRevision returns the schema and revision that mapped the event, if
// a schema did.
func (e Event) SchemaRevision() (string, int64, bool) {
	revision, err := strconv.ParseInt(e.Metadata[SchemaRevisionKey], 10, 64)
	return e.Metadata[SchemaKey], revision, err == nil && e.Metadata[SchemaKey] != ""
}

// Headline returns the first non-empty line of the content, for one-line
// terminal rendering.
func (e Event) Headline() string {
	for _, line := range strings.Split(e.Content, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
