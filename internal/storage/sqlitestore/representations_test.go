package sqlitestore

import (
	"context"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

var printImage = storage.EntityKey{Type: entity.TypeImage, Key: "ab12"}

// pendingImageFile is a file event holding the image printImage names.
func pendingImageFile(uid, path string) event.Event {
	metadata := event.File{Path: path}.Metadata()
	for key, value := range (event.Image{SHA256: printImage.Key, Status: event.CaptionPending}).Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: uid, Source: event.SourceFile, Timestamp: baseTime, Content: path, Metadata: metadata}
}

func description(text string, revision int) representation.Representation {
	return representation.Representation{Modality: representation.ModalityText, Kind: representation.KindDescription, Model: "qwen", Revision: revision, Content: text}
}

func TestSaveRepresentationReplacesTheSameKindInPlace(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, pendingImageFile("f1", "/home/eu/erro.png"), nil)
	first, err := store.SaveRepresentation(context.Background(), printImage, description("um terminal", 1))
	testcheck.NoError(t, err)
	second, err := store.SaveRepresentation(context.Background(), printImage, description("um terminal com erro", 2))
	testcheck.NoError(t, err)
	visible := representation.Representation{Modality: representation.ModalityText, Kind: representation.KindVisibleText, Model: "qwen", Revision: 2, Content: "panic"}
	_, err = store.SaveRepresentation(context.Background(), printImage, visible)
	testcheck.NoError(t, err)
	stored, err := store.RepresentationsOf(context.Background(), printImage)
	testcheck.NoError(t, err)
	if first != second || len(stored) != 2 || stored[0].Content != "um terminal com erro" || stored[0].Revision != 2 || stored[1].Kind != representation.KindVisibleText {
		t.Fatalf("expected the description replaced in place next to the visible text, got ids %d %d and %+v", first, second, stored)
	}
}

func TestSaveRepresentationOfUnknownEntityNamesIt(t *testing.T) {
	store := openTestStore(t)
	_, err := store.SaveRepresentation(context.Background(), printImage, description("nada", 1))
	if err == nil || !strings.Contains(err.Error(), `"ab12"`) {
		t.Fatalf("expected an error naming the image, got %v", err)
	}
}

func TestRepresentationGoesWithTheImagesLastCopy(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, pendingImageFile("f1", "/home/eu/erro.png"), nil)
	mustSave(t, store, pendingImageFile("f2", "/home/eu/copia.png"), nil)
	_, err := store.SaveRepresentation(context.Background(), printImage, description("um terminal", 1))
	testcheck.NoError(t, err)
	for i, uid := range []string{"f1", "f2"} {
		_, err := store.DeleteEvent(context.Background(), uid)
		testcheck.NoError(t, err)
		stored, err := store.RepresentationsOf(context.Background(), printImage)
		testcheck.NoError(t, err)
		if want := 1 - i; len(stored) != want {
			t.Fatalf("after forgetting %s expected %d representations, got %+v", uid, want, stored)
		}
	}
}
