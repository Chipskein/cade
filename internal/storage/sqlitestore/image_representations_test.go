package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/testcheck"
)

func storedImageKeys(t *testing.T, store *Store, uid string) event.Image {
	t.Helper()
	ev, found, err := store.StoredEvent(context.Background(), uid)
	if err != nil || !found {
		t.Fatalf("expected %q stored, got found=%v err=%v", uid, found, err)
	}
	return ev.Image()
}

func TestDescribedImageTextLeavesTheMetadataForRepresentations(t *testing.T) {
	store := openTestStore(t)
	ev := imageFileEvent("/prints/erro.png", "h1", event.CaptionDescribed, "panic: nil map")
	mustSave(t, store, ev, nil)
	if kept := storedImageKeys(t, store, ev.UID); kept.Description != "" || kept.Model != "" || kept.SHA256 != "h1" || kept.Status != event.CaptionDescribed {
		t.Fatalf("expected only status and hash in the metadata, got %+v", kept)
	}
	stored, err := store.RepresentationsOf(context.Background(), imageOwner("h1"))
	testcheck.NoError(t, err)
	if len(stored) != 2 || stored[0].Kind != representation.KindDescription || stored[0].Content != "um terminal" || stored[0].Revision != 1 || stored[1].Content != "panic: nil map" {
		t.Fatalf("expected the description and the visible text as representations, got %+v", stored)
	}
	if stored, _, _ := store.StoredEvent(context.Background(), ev.UID); stored.Content != ev.Content {
		t.Fatal("expected the event text, which the search reads, unchanged")
	}
}

// A picture pasted in Teams is described like a file (#24).
func TestADescribedPictureInAMessageIsRepresentedToo(t *testing.T) {
	store := openTestStore(t)
	message := event.Message{Sender: "Ana Souza", Kind: event.KindChat}.Metadata()
	for key, value := range (event.Image{SHA256: "h2", Status: event.CaptionDescribed, Description: "print do erro 502"}).Metadata() {
		message[key] = value
	}
	mustSave(t, store, event.Event{UID: "teams-1", Source: event.SourceTeams, Timestamp: baseTime, Content: "Ana Souza: \nImagem: print do erro 502", Metadata: message}, nil)
	stored, err := store.RepresentationsOf(context.Background(), imageOwner("h2"))
	if err != nil || len(stored) != 1 || stored[0].Content != "print do erro 502" {
		t.Fatalf("expected the picture's description as a representation, got %+v (err %v)", stored, err)
	}
}

func TestDescribingAgainReplacesAndDropsTheVisibleText(t *testing.T) {
	store := openTestStore(t)
	ev := imageFileEvent("/prints/erro.png", "h1", event.CaptionDescribed, "panic: nil map")
	mustSave(t, store, ev, nil)
	again := imageFileEvent("/prints/erro.png", "h1", event.CaptionDescribed, "")
	again.Metadata[event.CaptionPromptVersionKey] = "2"
	testcheck.NoError(t, store.UpdateEvent(context.Background(), again, whole(again, nil)))
	image, found, err := store.DescribedImage(context.Background(), "h1")
	if err != nil || !found || image.PromptVersion != 2 || image.VisibleText != "" || image.Description != "um terminal" {
		t.Fatalf("expected the new description without visible text, got %+v %v (err %v)", image, found, err)
	}
}

func TestCopiesOfAnImageShareOneDescription(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, imageFileEvent("/prints/a.png", "h1", event.CaptionDescribed, ""), nil)
	mustSave(t, store, imageFileEvent("/backup/a.png", "h1", event.CaptionDescribed, ""), nil)
	var images, descriptions int
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM entities WHERE type = ?`, string(entity.TypeImage)).Scan(&images))
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM representations`).Scan(&descriptions))
	if images != 1 || descriptions != 1 {
		t.Fatalf("expected one Image with one description, got %d and %d", images, descriptions)
	}
}

func TestImageTextMigrationMovesWhatTheMetadataHeld(t *testing.T) {
	store := openTestStore(t)
	ev := imageFileEvent("/prints/erro.png", "h1", event.CaptionDescribed, "panic: nil map")
	mustSave(t, store, ev, nil)
	metadata, err := encodeMetadata(ev.Metadata)
	testcheck.NoError(t, err)
	for _, statement := range []string{`DELETE FROM representations`, `DELETE FROM event_entities WHERE role = 'shows'`, `UPDATE events SET metadata = '` + metadata + `'`, `PRAGMA user_version = 17`} {
		_, err := store.db.Exec(statement)
		testcheck.NoError(t, err)
	}
	testcheck.NoError(t, migrate(context.Background(), store.db, filepath.Join(t.TempDir(), "cade.db"), schemaMigrations, Hooks{}))
	image, found, err := store.DescribedImage(context.Background(), "h1")
	if err != nil || !found || image.Description != "um terminal" || image.VisibleText != "panic: nil map" || image.Model != ev.Image().Model {
		t.Fatalf("expected the description moved to representations, got %+v %v (err %v)", image, found, err)
	}
	if kept := storedImageKeys(t, store, ev.UID); kept.Description != "" || versionOf(t, store.db) != latestVersion() {
		t.Fatalf("expected the metadata rewritten without the text at v%d, got %+v", latestVersion(), kept)
	}
}
