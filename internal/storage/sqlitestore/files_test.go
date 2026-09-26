package sqlitestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func fileVersionEvent(path, text string, modified time.Time) event.Event {
	return event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile, Timestamp: modified, Content: text,
		Metadata: event.File{Path: path, Size: int64(len(text)), ModifiedAt: modified}.Metadata()}
}

var fileDay = time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)

func TestSaveAndUpdateRecordEachFileVersion(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, fileVersionEvent("/notas/a.md", "v1", fileDay), nil)
	store.UpdateEvent(ctx, fileVersionEvent("/notas/a.md", "v2", fileDay.Add(time.Hour)), nil)
	modifications, err := store.FileModificationsBetween(ctx, fileDay, fileDay.Add(24*time.Hour))
	if err != nil || len(modifications) != 2 || !modifications[1].ModifiedAt.Equal(fileDay.Add(time.Hour)) || modifications[1].Size != 2 {
		t.Fatalf("expected both versions in the history, got %+v (err %v)", modifications, err)
	}
}

func TestMarkMissingFilesFlagsAndClears(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, fileVersionEvent("/notas/a.md", "a", fileDay), nil)
	mustSave(t, store, fileVersionEvent("/notas/sub/b.md", "b", fileDay), nil)
	mustSave(t, store, fileVersionEvent("/outra/c.md", "c", fileDay), nil)
	removed, err := store.MarkMissingFiles(ctx, "/notas", map[string]bool{"/notas/a.md": true}, fileDay)
	gone, _, _ := store.StoredEvent(ctx, event.StableID(event.SourceFile, "/notas/sub/b.md"))
	other, _, _ := store.StoredEvent(ctx, event.StableID(event.SourceFile, "/outra/c.md"))
	if err != nil || removed != 1 || !gone.File().RemovedAt.Equal(fileDay) || !other.File().RemovedAt.IsZero() {
		t.Fatalf("expected only the missing file under the root flagged, got %d (err %v)", removed, err)
	}
	store.MarkMissingFiles(ctx, "/notas", map[string]bool{"/notas/a.md": true, "/notas/sub/b.md": true}, fileDay)
	back, _, _ := store.StoredEvent(ctx, event.StableID(event.SourceFile, "/notas/sub/b.md"))
	if !back.File().RemovedAt.IsZero() {
		t.Fatal("expected a returning file unflagged")
	}
}

// Migration 4 turns one event per version into one per path.
func TestCollapseFileVersionsMigration(t *testing.T) {
	legacy := newLegacyDatabase(t, 3)
	for i, text := range []string{"v1", "v2", "v3"} {
		old := fileVersionEvent("/notas/a.md", text, fileDay.Add(time.Duration(i)*time.Hour))
		old.UID = old.UID + text
		legacy.add(old, []float32{float32(i), 1})
	}
	legacy.close()
	var backups []string
	migrated, err := OpenWithHooks(context.Background(), legacy.Path, Hooks{BackupCreated: func(p string) { backups = append(backups, p) }})
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	current, found, _ := migrated.StoredEvent(context.Background(), event.StableID(event.SourceFile, "/notas/a.md"))
	var events, chunks int
	migrated.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&events)
	migrated.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&chunks)
	history, _ := migrated.FileModificationsBetween(context.Background(), fileDay, fileDay.Add(24*time.Hour))
	if !found || current.Content != "v3" || events != 1 || chunks != 1 || len(history) != 3 || len(backups) != 1 {
		t.Fatalf("expected v3 kept alone with 3 versions in history and a backup, got %q, %d events, %d chunks, %d versions, %v", current.Content, events, chunks, len(history), backups)
	}
	if revision, _ := current.Revision(); revision != fileDay.Add(2*time.Hour).UnixNano() {
		t.Fatalf("expected the latest modification as revision, got %d", revision)
	}
}

// Privacy: the edit history holds paths, and forget must erase it.
func TestForgetFileErasesHistory(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, fileVersionEvent("/notas/segredo.md", "a", fileDay), nil)
	store.DeleteSource(ctx, event.SourceFile)
	if history, _ := store.FileModificationsBetween(ctx, fileDay, fileDay.Add(time.Hour)); len(history) != 0 {
		t.Fatalf("expected no history left, got %+v", history)
	}
}

// Acceptance (phase 1, see CHANGELOG.md): ten versions of a 200 KB file
// take about the space of one.
func TestTenVersionsTakeTheSpaceOfOne(t *testing.T) {
	sizeAfter := func(versions int) int64 {
		path := filepath.Join(t.TempDir(), "cade.db")
		store, _ := Open(context.Background(), path)
		text := string(make([]byte, 200*1024))
		for i := range versions {
			version := fileVersionEvent("/notas/grande.md", text+string(rune('a'+i)), fileDay.Add(time.Duration(i)*time.Hour))
			if i == 0 {
				mustSave(t, store, version, nil)
			} else {
				store.UpdateEvent(context.Background(), version, nil)
			}
		}
		store.compact(context.Background())
		store.Close()
		info, _ := os.Stat(path)
		return info.Size()
	}
	one, ten := sizeAfter(1), sizeAfter(10)
	if float64(ten) > 1.2*float64(one) {
		t.Fatalf("expected ten versions near one version's size, got %d vs %d bytes", ten, one)
	}
}
