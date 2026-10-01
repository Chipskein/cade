package sqlitestore

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

func noteEvent(uid, path, text string) event.Event {
	return event.Event{UID: uid, Source: event.SourceFile, Timestamp: baseTime, Content: text,
		Metadata: event.File{Path: path, Size: int64(len(text))}.Metadata()}
}

func TestEventIdentifierIsTheHashOrThePath(t *testing.T) {
	cases := map[string]event.Event{
		"e5f6a7b8c9":    commitEvent("a", "corrige", "e5f6a7b8c9"),
		"/notas/erp.md": noteEvent("b", "/notas/erp.md", "retry"),
		"":              sampleEvent("c", event.SourceBrowser, 0),
	}
	for want, ev := range cases {
		if got := eventIdentifier(ev); got != want {
			t.Errorf("eventIdentifier(%s event) = %q, want %q", ev.Source, got, want)
		}
	}
}

// Two commits with the same message share a text (#67), not a hash.
func TestHashFindsOnlyItsCommit(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, commitEvent("a", "Merge branch 'dev'", "e5f6a7b8c9"), []float32{1, 0})
	mustSave(t, store, commitEvent("b", "Merge branch 'dev'", "a1b2c3d4e5"), []float32{1, 0})
	if got := lexicalUIDs(t, store, `a1b2c3*`); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("expected only commit b by its hash, got %v", got)
	}
	if got := lexicalUIDs(t, store, `"merge"`); len(got) != 2 {
		t.Fatalf("expected both commits by their message, got %v", got)
	}
	if rows := ftsRows(t, store, `chunks_fts`, `e5f6a7*`); rows != 0 {
		t.Fatalf("expected no hash in the chunks' index, got %d rows", rows)
	}
}

func ftsRows(t *testing.T, store *Store, table, match string) int {
	t.Helper()
	var rows int
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+table+` MATCH ?`, match).Scan(&rows))
	return rows
}

func TestEditedPathIsFoundAndTheOldOneIsNot(t *testing.T) {
	store := openTestStore(t)
	note := noteEvent("n", "/notas/antigo.md", "retry do ERP")
	mustSave(t, store, note, []float32{1, 0})
	moved := noteEvent("n", "/notas/novo.md", "retry do ERP")
	testcheck.NoError(t, store.UpdateEvent(context.Background(), moved, whole(moved, []float32{1, 0})))
	if got := lexicalUIDs(t, store, `"antigo"`); len(got) != 0 {
		t.Fatalf("expected the old path gone, got %v", got)
	}
	if got := lexicalUIDs(t, store, `"novo"`); !slices.Equal(got, []string{"n"}) {
		t.Fatalf("expected the note by its new path, got %v", got)
	}
}

// Privacy: forget and `forget <source>` leave no identifier searchable.
func TestForgetRemovesIdentifiers(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, commitEvent("a", "corrige", "e5f6a7b8c9"), []float32{1, 0})
	mustSave(t, store, commitEvent("b", "ajusta", "a1b2c3d4e5"), []float32{0, 1})
	_, err := store.DeleteEvent(ctx, "a")
	testcheck.NoError(t, err)
	if rows := ftsRows(t, store, `event_identifiers_fts`, `e5f6a7*`); rows != 0 {
		t.Fatalf("expected the forgotten commit's hash gone, got %d rows", rows)
	}
	_, err = store.DeleteSource(ctx, event.SourceGit)
	testcheck.NoError(t, err)
	var rows int
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM event_identifiers_fts`).Scan(&rows))
	if rows != 0 {
		t.Fatalf("expected no identifiers after forgetting git, got %d rows", rows)
	}
}

// migrateTo applies the migrations up to version, and no further, to the
// database at path: a later step then meets the schema of its time.
func migrateTo(t *testing.T, path string, version int) *sql.DB {
	t.Helper()
	db := openRaw(t, path)
	testcheck.NoError(t, migrate(context.Background(), db, path, schemaMigrations[:version], Hooks{}))
	return db
}

// A database at version 11 has the hash in its first chunk's entry;
// migration 12 moves it to the event.
func TestMoveIdentifiersToEventsMigration(t *testing.T) {
	legacy := newLegacyDatabase(t, 4)
	legacy.add(commitEvent("a", "corrige retry", "e5f6a7b8c9"), []float32{1, 0})
	legacy.close()
	db := migrateTo(t, legacy.Path, 11)
	for _, statement := range []string{`DELETE FROM chunks_fts WHERE rowid = 1`,
		`INSERT INTO chunks_fts (rowid, text) VALUES (1, 'corrige retry` + "\n" + `e5f6a7b8c9')`} {
		_, err := db.Exec(statement)
		testcheck.NoError(t, err)
	}
	db.Close()
	migrated, err := Open(context.Background(), legacy.Path)
	testcheck.NoError(t, err)
	defer migrated.Close()
	if got := lexicalUIDs(t, migrated, `e5f6a7*`); !slices.Equal(got, []string{"a"}) || ftsRows(t, migrated, `chunks_fts`, `e5f6a7*`) != 0 {
		t.Fatalf("expected the hash found through the event only, got %v", got)
	}
	if got := lexicalUIDs(t, migrated, `"retry"`); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("expected the text still indexed, got %v", got)
	}
}

func TestBestLexicalHitsMergesByScoreAndKeepsLimit(t *testing.T) {
	hit := func(uid string, score float64) scoredLexicalHit {
		return scoredLexicalHit{hit: storage.ScoredEvent{Event: event.Event{UID: uid}}, score: score}
	}
	got := bestLexicalHits([]scoredLexicalHit{hit("texto", -1), hit("pior", -0.5), hit("hash", -3)}, 2)
	if len(got) != 2 || got[0].Event.UID != "hash" || got[1].Event.UID != "texto" {
		t.Fatalf("expected [hash texto], got %+v", got)
	}
}
