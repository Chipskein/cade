package testfakes

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
)

func TestFakeRepresentationsFollowTheirEntity(t *testing.T) {
	store := NewFakeEventStore()
	owner := storage.EntityKey{Type: entity.TypeImage, Key: "ab12"}
	rep := representation.Representation{Modality: representation.ModalityText, Kind: representation.KindDescription, Model: "qwen", Content: "um terminal"}
	if _, err := store.SaveRepresentation(context.Background(), owner, rep); err == nil {
		t.Fatal("expected an unknown entity refused")
	}
	metadata := event.File{Path: "/a.png"}.Metadata()
	metadata[event.ImageSHA256Key] = "ab12"
	store.Events = []event.Event{{UID: "f1", Source: event.SourceFile, Metadata: metadata}}
	first, _ := store.SaveRepresentation(context.Background(), owner, rep)
	rep.Content = "um terminal com erro"
	second, err := store.SaveRepresentation(context.Background(), owner, rep)
	stored, _ := store.RepresentationsOf(context.Background(), owner)
	if err != nil || first != second || len(stored) != 1 || stored[0].Content != "um terminal com erro" {
		t.Fatalf("expected one replaced representation, got %+v (ids %d %d, err %v)", stored, first, second, err)
	}
	store.Events = nil
	if gone, _ := store.RepresentationsOf(context.Background(), owner); len(gone) != 0 {
		t.Fatalf("expected none once no event refers to the image, got %+v", gone)
	}
}
