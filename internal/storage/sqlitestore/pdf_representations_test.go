package sqlitestore

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/testcheck"
)

func scannedPDF(path string, pages ...event.PDFPageImage) event.Event {
	modified := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	metadata := event.File{Path: path, ModifiedAt: modified}.Metadata()
	maps.Copy(metadata, event.PDF{Status: event.PDFRead, Pages: len(pages)}.Metadata())
	maps.Copy(metadata, event.PDFPageImagesMetadata(pages))
	return event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile, Timestamp: modified, Content: "scan.pdf\nImagem: um contrato", Metadata: metadata}
}

func contractPage(number int, hash string) event.PDFPageImage {
	return event.PDFPageImage{Number: number, Image: event.Image{SHA256: hash, Status: event.CaptionDescribed,
		Description: "um contrato", VisibleText: "CLÁUSULA 1", Model: "qwen", PromptVersion: 2}}
}

func TestAScannedPageIsRepresentedAndItsTextLeavesTheEvent(t *testing.T) {
	store := openTestStore(t)
	ev := scannedPDF("/docs/scan.pdf", contractPage(1, "p1"))
	mustSave(t, store, ev, nil)
	stored, err := store.RepresentationsOf(context.Background(), imageOwner("p1"))
	testcheck.NoError(t, err)
	if len(stored) != 2 || stored[0].Kind != representation.KindDescription || stored[0].Content != "um contrato" || stored[1].Content != "CLÁUSULA 1" {
		t.Fatalf("representations = %+v", stored)
	}
	kept, _, err := store.StoredEvent(context.Background(), ev.UID)
	testcheck.NoError(t, err)
	pages := kept.PDFPageImages()
	if len(pages) != 1 || pages[0].Number != 1 || pages[0].Image.SHA256 != "p1" || pages[0].Image.Description != "" {
		t.Errorf("stored pages = %+v, want the hash without the text", pages)
	}
}

func TestDescribedImageFindsAScannedPageByHash(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, scannedPDF("/docs/scan.pdf", contractPage(1, "p1")), nil)
	image, found, err := store.DescribedImage(context.Background(), "p1")
	testcheck.NoError(t, err)
	if !found || image.Description != "um contrato" || image.VisibleText != "CLÁUSULA 1" || image.Status != event.CaptionDescribed {
		t.Errorf("image = %+v, found = %v", image, found)
	}
	if _, found, err := store.DescribedImage(context.Background(), "unknown"); err != nil || found {
		t.Errorf("unknown hash: found = %v, %v", found, err)
	}
}

func TestPagesAreNotAmongTheImagesAFileShows(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, scannedPDF("/docs/scan.pdf", contractPage(1, "p1")), nil)
	outdated, err := store.OutdatedImages(context.Background(), "other-model", 9)
	testcheck.NoError(t, err)
	if len(outdated) != 0 {
		t.Errorf("outdated = %+v, a PDF's page is not an image file to describe again", outdated)
	}
	var role string
	err = store.db.QueryRow(`SELECT role FROM event_entities JOIN entities ON entities.id = entity_id WHERE entity_key = ?`, "p1").Scan(&role)
	testcheck.NoError(t, err)
	if role != string(entity.RolePageImage) {
		t.Errorf("role = %q", role)
	}
}

// PRIVACY: forgetting a PDF leaves nothing of what its scanned pages showed.
func TestForgetRemovesWhatTheScannedPagesShowed(t *testing.T) {
	forgetters := map[string]func(*Store, event.Event) error{
		"by event": func(store *Store, ev event.Event) error {
			_, err := store.DeleteEvent(context.Background(), ev.UID)
			return err
		},
		"by source": func(store *Store, _ event.Event) error {
			_, err := store.DeleteSource(context.Background(), event.SourceFile)
			return err
		},
	}
	for name, forget := range forgetters {
		store := openTestStore(t)
		ev := scannedPDF("/docs/scan.pdf", contractPage(1, "p1"))
		mustSave(t, store, ev, nil)
		testcheck.NoError(t, forget(store, ev))
		var left int
		testcheck.NoError(t, store.db.QueryRow(`SELECT count(*) FROM representations`).Scan(&left))
		if left != 0 {
			t.Errorf("%s: %d representations left", name, left)
		}
		if _, found, err := store.DescribedImage(context.Background(), "p1"); err != nil || found {
			t.Errorf("%s: description still found (%v)", name, err)
		}
		if _, found, err := store.StoredEvent(context.Background(), ev.UID); err != nil || found {
			t.Errorf("%s: event still found (%v)", name, err)
		}
	}
}

func TestAPageSharedByTwoPDFsOutlivesForgettingOne(t *testing.T) {
	store := openTestStore(t)
	first := scannedPDF("/docs/a.pdf", contractPage(1, "p1"))
	second := scannedPDF("/docs/b.pdf", contractPage(4, "p1"))
	mustSave(t, store, first, nil)
	mustSave(t, store, second, nil)
	_, err := store.DeleteEvent(context.Background(), first.UID)
	testcheck.NoError(t, err)
	if _, found, err := store.DescribedImage(context.Background(), "p1"); err != nil || !found {
		t.Errorf("the other PDF still holds the page: found = %v, %v", found, err)
	}
}
