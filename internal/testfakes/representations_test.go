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

func TestFakeRepresentationVectorsRankByCosine(t *testing.T) {
	store := NewFakeEventStore()
	owner := storage.EntityKey{Type: entity.TypeImage, Key: "ab12"}
	metadata := event.File{Path: "/a.png"}.Metadata()
	metadata[event.ImageSHA256Key] = "ab12"
	store.Events = []event.Event{{UID: "f1", Source: event.SourceFile, Metadata: metadata}}
	rep := representation.Representation{Modality: representation.ModalityVector, Kind: representation.KindEmbedding, Model: "clip", Content: "v1"}
	id, _ := store.SaveRepresentation(context.Background(), owner, rep)
	if err := store.SaveRepresentationVector(context.Background(), id, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	hits, err := store.NearestRepresentations(context.Background(), []float32{1, 0}, representation.KindEmbedding, 3)
	if err != nil || len(hits) != 1 || hits[0].Owner != owner || hits[0].Distance > 1e-9 {
		t.Fatalf("expected the embedding at distance 0, got %+v (err %v)", hits, err)
	}
	rep.Content = "v2"
	_, _ = store.SaveRepresentation(context.Background(), owner, rep)
	if hits, _ := store.NearestRepresentations(context.Background(), []float32{1, 0}, representation.KindEmbedding, 3); len(hits) != 0 {
		t.Fatalf("expected the vector dropped with new content, got %+v", hits)
	}
}
