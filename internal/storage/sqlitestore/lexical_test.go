package sqlitestore

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

func commitEvent(uid, message, hash string) event.Event {
	return event.Event{UID: uid, Source: event.SourceGit, Timestamp: baseTime, Content: message,
		Metadata: event.Commit{Repository: "/src/api", Hash: hash}.Metadata()}
}

func lexicalUIDs(t *testing.T, store *Store, match string) []string {
	t.Helper()
	hits, err := store.SearchLexical(context.Background(), storage.LexicalQuery{Match: match, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var uids []string
	for _, hit := range hits {
		uids = append(uids, hit.Event.UID)
	}
	return uids
}

func TestSearchLexicalFindsTextAndIdentifiers(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, commitEvent("a", "PROJ-481: valida CEP da transportadora", "e5f6a7b8c9"), []float32{1, 0})
	mustSave(t, store, commitEvent("b", "PROJ-418: ajusta filtro de data", "a1b2c3d4e5"), []float32{0, 1})
	if got := lexicalUIDs(t, store, `"proj 481"`); len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected only PROJ-481, got %v", got)
	}
	if got := lexicalUIDs(t, store, `e5f6a7b*`); len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected the commit by hash prefix, got %v", got)
	}
	if got := lexicalUIDs(t, store, `"transportadora"`); len(got) != 1 {
		t.Fatalf("expected a word match, got %v", got)
	}
}

// Privacy: forget, an edit and reindex must leave no searchable terms.
func TestDeletedTextIsNotSearchable(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	secret := commitEvent("a", "senha do cofre 8472", "e5f6a7b8c9")
	mustSave(t, store, secret, []float32{1, 0})
	edited := secret
	edited.Content = "texto novo"
	testcheck.NoError(t, store.UpdateEvent(ctx, edited, whole(edited, []float32{1, 0})))
	if got := lexicalUIDs(t, store, `"cofre"`); len(got) != 0 {
		t.Fatalf("expected the old text gone after an edit, got %v", got)
	}
	testcheck.NoError(t, store.StartReindex(ctx, "outro.gguf"))
	if got := lexicalUIDs(t, store, `"novo"`); len(got) != 0 {
		t.Fatalf("expected no terms after reindex starts, got %v", got)
	}
	testcheck.NoError(t, store.SaveEmbeddings(ctx, []storage.EventEmbedding{{Event: edited, Chunks: whole(edited, []float32{1, 0})}}))
	if _, err := store.DeleteSource(ctx, event.SourceGit); err != nil {
		t.Fatal(err)
	}
	var rows int
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM chunks_fts`).Scan(&rows))
	if got := lexicalUIDs(t, store, `"novo"`); len(got) != 0 || rows != 0 {
		t.Fatalf("expected nothing searchable after forget, got %v and %d rows", got, rows)
	}
}

// Migration 6 indexes the chunks of databases created before it.
func TestIndexExistingChunksMigration(t *testing.T) {
	legacy := newLegacyDatabase(t, 4)
	legacy.add(commitEvent("a", "corrige retry do ERP", "e5f6a7b8c9"), []float32{1, 0})
	legacy.close()
	store, err := Open(context.Background(), legacy.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := lexicalUIDs(t, store, `"retry"`); len(got) != 1 {
		t.Fatalf("expected the migrated chunk indexed, got %v", got)
	}
}
