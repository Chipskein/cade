package sqlitestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

func TestInspectMissingDatabaseCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	state, err := Inspect(context.Background(), path)
	if err != nil || state.Exists || !state.FTS5 || state.LatestSchemaVersion != latestVersion() {
		t.Fatalf("expected a missing database with FTS5, got %+v %v", state, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Inspect must not create the database, stat says %v", err)
	}
}

func TestInspectCurrentDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.StartReindex(ctx, "nomic.gguf"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	state, err := Inspect(ctx, path)
	if err != nil || !state.Exists || state.SchemaVersion != latestVersion() || state.MigrationBackup {
		t.Fatalf("expected an up-to-date database, got %+v %v", state, err)
	}
	if state.EmbeddingModel != "nomic.gguf" || !state.ReindexPending || state.SizeBytes == 0 {
		t.Fatalf("expected the recorded model and a pending reindex, got %+v", state)
	}
}

// An old database with events reports the copy its migration will make,
// and inspecting it leaves the version where it was.
func TestInspectOldDatabaseLeavesItUnmigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	mustSave(t, store, sampleEvent("e1", event.SourceGit, 0), []float32{1, 0})
	store.Close()
	raw := openRaw(t, path)
	if _, err := raw.Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatal(err)
	}
	state, err := Inspect(context.Background(), path)
	if err != nil || state.SchemaVersion != 4 || !state.MigrationBackup {
		t.Fatalf("expected v4 with a pending backup, got %+v %v", state, err)
	}
	if version := versionOf(t, raw); version != 4 {
		t.Fatalf("Inspect migrated the database to v%d", version)
	}
}

func TestInspectNewerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	raw := openRaw(t, path)
	if _, err := raw.Exec(createEventsTable + `; PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	state, err := Inspect(context.Background(), path)
	if err != nil || state.SchemaVersion != 99 || state.MigrationBackup {
		t.Fatalf("expected v99 without a backup, got %+v %v", state, err)
	}
}

func TestInspectEmptyFileIsVersionZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := Inspect(context.Background(), path)
	if err != nil || !state.Exists || state.SchemaVersion != 0 {
		t.Fatalf("expected an existing database at v0, got %+v %v", state, err)
	}
}

// doctor reads the empty share read-only, to suggest `cade compact`.
func TestInspectMeasuresTheVectorBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	saveVectorEvents(t, store, 2)
	store.Close()
	state, err := Inspect(context.Background(), path)
	if err != nil || state.VectorSlots != (storage.VectorSlots{Slots: vec0BlockSize, Vectors: 2}) {
		t.Fatalf("expected one block holding 2 vectors, got %+v: %v", state.VectorSlots, err)
	}
}
