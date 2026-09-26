package sqlitestore

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/chipskein/cade/internal/event"
)

// legacyDatabase builds a database in the schema of an old version, so a
// migration is tested on what it will really meet, not on today's schema:
// one vector per event in event_embeddings, as before migration 5.
type legacyDatabase struct {
	t    *testing.T
	db   *sql.DB
	Path string
}

func newLegacyDatabase(t *testing.T, version int) *legacyDatabase {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cade.db")
	legacy := &legacyDatabase{t: t, db: openRaw(t, path), Path: path}
	legacy.exec(createEventsTable)
	legacy.exec(`ALTER TABLE events ADD COLUMN content_hash TEXT`)
	legacy.exec(`CREATE VIRTUAL TABLE event_embeddings USING vec0(event_id INTEGER PRIMARY KEY, embedding float[2] distance_metric=cosine, source TEXT, occurred_at INTEGER)`)
	legacy.exec(`INSERT INTO store_settings (key, value) VALUES ('embedding_dimensions', '2')`)
	if version >= 4 {
		legacy.exec(createFileModifications)
	}
	legacy.exec(fmt.Sprintf(`PRAGMA user_version = %d`, version))
	return legacy
}

func (l *legacyDatabase) exec(query string, args ...any) {
	l.t.Helper()
	if _, err := l.db.Exec(query, args...); err != nil {
		l.t.Fatalf("legacy schema: %v", err)
	}
}

// add stores ev with its vector the old way and returns its row id.
func (l *legacyDatabase) add(ev event.Event, vector []float32) int64 {
	l.t.Helper()
	metadata, _ := encodeMetadata(ev.Metadata)
	result, err := l.db.Exec(`INSERT INTO events (uid, occurred_at, source, content, metadata, content_hash) VALUES (?, ?, ?, ?, ?, ?)`,
		ev.UID, toUnixMillis(ev.Timestamp), string(ev.Source), ev.Content, metadata, contentHash(ev.Content))
	if err != nil {
		l.t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	if vector != nil {
		blob, _ := sqlitevec.SerializeFloat32(vector)
		l.exec(`INSERT INTO event_embeddings (event_id, embedding, source, occurred_at) VALUES (?, ?, ?, ?)`, id, blob, string(ev.Source), toUnixMillis(ev.Timestamp))
	}
	return id
}

func (l *legacyDatabase) close() {
	l.db.Close()
}
