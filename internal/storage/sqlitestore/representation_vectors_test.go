package sqlitestore

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/testcheck"
)

func embedding(rep representation.Representation) representation.Representation {
	rep.Modality, rep.Kind = representation.ModalityVector, representation.KindEmbedding
	return rep
}

// storeWithVector saves one image, its description and an embedding with
// a vector, and returns the embedding's id.
func storeWithVector(t *testing.T) (*Store, int64) {
	t.Helper()
	store := openTestStore(t)
	mustSave(t, store, pendingImageFile("f1", "/home/eu/erro.png"), nil)
	_, err := store.SaveRepresentation(context.Background(), printImage, description("um terminal", 1))
	testcheck.NoError(t, err)
	id, err := store.SaveRepresentation(context.Background(), printImage, embedding(description("clip", 1)))
	testcheck.NoError(t, err)
	testcheck.NoError(t, store.SaveRepresentationVector(context.Background(), id, []float32{1, 0, 0}))
	return store, id
}

func nearestEmbeddings(t *testing.T, store *Store) int {
	t.Helper()
	hits, err := store.NearestRepresentations(context.Background(), []float32{1, 0.1, 0}, representation.KindEmbedding, 5)
	testcheck.NoError(t, err)
	return len(hits)
}

func TestRepresentationVectorIsFoundByItsKind(t *testing.T) {
	store, id := storeWithVector(t)
	hits, err := store.NearestRepresentations(context.Background(), []float32{1, 0.1, 0}, representation.KindEmbedding, 5)
	testcheck.NoError(t, err)
	if len(hits) != 1 || hits[0].ID != id || hits[0].Owner != printImage || hits[0].Distance > 0.01 {
		t.Fatalf("expected the image's embedding, close, got %+v", hits)
	}
	if other, _ := store.NearestRepresentations(context.Background(), []float32{1, 0, 0}, representation.KindDescription, 5); len(other) != 0 {
		t.Fatalf("expected no vector of another kind, got %+v", other)
	}
}

func TestReplacingTheContentDropsTheVector(t *testing.T) {
	store, _ := storeWithVector(t)
	_, err := store.SaveRepresentation(context.Background(), printImage, embedding(description("clip", 1)))
	testcheck.NoError(t, err)
	if nearestEmbeddings(t, store) != 1 {
		t.Fatal("expected the vector kept when the content is unchanged")
	}
	_, err = store.SaveRepresentation(context.Background(), printImage, embedding(description("clip v2", 2)))
	testcheck.NoError(t, err)
	if nearestEmbeddings(t, store) != 0 {
		t.Fatal("expected the stale vector dropped with the new content")
	}
}

func TestForgetDropsTheVectorThroughTheCascade(t *testing.T) {
	store, _ := storeWithVector(t)
	_, err := store.DeleteEvent(context.Background(), "f1")
	testcheck.NoError(t, err)
	var vectors int
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM representation_vectors`).Scan(&vectors))
	if vectors != 0 {
		t.Fatalf("expected the vector gone with the image's last copy, got %d", vectors)
	}
}

func TestRepresentationVectorsKeepOneDimension(t *testing.T) {
	store, id := storeWithVector(t)
	if err := store.SaveRepresentationVector(context.Background(), id, []float32{1, 0}); err == nil {
		t.Fatal("expected a 2-dimension vector refused next to 3-dimension ones")
	}
	if err := store.SaveRepresentationVector(context.Background(), 999, []float32{1, 0, 0}); err == nil {
		t.Fatal("expected an unknown representation refused")
	}
}

func TestNearestRepresentationsBeforeAnyVectorIsEmpty(t *testing.T) {
	store := openTestStore(t)
	if nearestEmbeddings(t, store) != 0 {
		t.Fatal("expected nothing before the vector table exists")
	}
}
