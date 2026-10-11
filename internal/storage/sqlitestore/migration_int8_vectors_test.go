package sqlitestore

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// float32SchemaVersion is the last schema with float32 vectors.
const float32SchemaVersion = 10

// sameDirectionCosine is how far int8 rounding may turn a 2-dim test
// vector: about 1/254 per component.
const sameDirectionCosine = 0.9999

// sameDirection reports whether a and b point the same way, within what
// the int8 rounding moves a vector.
func sameDirection(a, b []float32) bool {
	var dot, normA, normB float64
	for i := range min(len(a), len(b)) {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	return len(a) == len(b) && dot/math.Sqrt(normA*normB) > sameDirectionCosine
}

// float32Database is a database at version 10, with float32 vectors.
func float32Database(t *testing.T, vectors map[string][]float32) string {
	t.Helper()
	legacy := newLegacyDatabase(t, 4)
	for uid, vector := range vectors {
		legacy.add(sampleEvent(uid, event.SourceGit, 0), vector)
	}
	steps := schemaMigrations[:float32SchemaVersion]
	testcheck.NoError(t, migrate(context.Background(), legacy.db, legacy.Path, steps, Hooks{}))
	legacy.close()
	return legacy.Path
}

func TestConvertVectorsToInt8KeepsDirectionsAndNearest(t *testing.T) {
	vectors := map[string][]float32{"near": {0.9, 0.1}, "far": {-0.2, 0.7}}
	var backups []string
	store, err := OpenWithHooks(context.Background(), float32Database(t, vectors), Hooks{BackupCreated: func(p string) { backups = append(backups, p) }})
	testcheck.NoError(t, err)
	defer store.Close()
	stored := firstVectors(t, store, "near", "far")
	if !sameDirection(stored["near"], vectors["near"]) || !sameDirection(stored["far"], vectors["far"]) {
		t.Fatalf("expected the float32 directions kept, got %v", stored)
	}
	hits, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 2})
	if err != nil || len(hits) != 2 || hits[0].Event.UID != "near" {
		t.Fatalf("expected near first, got %+v: %v", hits, err)
	}
	if len(backups) != 1 || !strings.Contains(backups[0], ".before-v11-") {
		t.Fatalf("expected one backup before v11, got %v", backups)
	}
}

func TestConvertVectorsToInt8StoresOneBytePerDimension(t *testing.T) {
	store, err := Open(context.Background(), float32Database(t, map[string][]float32{"a": {0.3, -0.4}}))
	testcheck.NoError(t, err)
	defer store.Close()
	var bytes int
	testcheck.NoError(t, store.db.QueryRow(`SELECT length(embedding) FROM chunk_embeddings`).Scan(&bytes))
	if bytes != 2 {
		t.Fatalf("expected 2 bytes for 2 int8 dimensions, got %d", bytes)
	}
}

func TestFloat32BlobToInt8RejectsATruncatedBlob(t *testing.T) {
	_, err := float32BlobToInt8([]byte{1, 2, 3})
	if err == nil || !strings.Contains(err.Error(), "3 bytes") {
		t.Fatalf("expected an error naming the 3-byte blob, got %v", err)
	}
}
