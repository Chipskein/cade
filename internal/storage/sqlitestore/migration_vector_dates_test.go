package sqlitestore

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// occurredAtRewrite turns today's table back into the layout of versions
// 11 and 12, to test migration 13 on what it will really meet.
var occurredAtRewrite = vectorRewrite{componentBytes: int8Bytes, convert: keepVectorBlob,
	readMetadata: `source, first_at`, layout: occurredAtLayout}

// storeAtVersion12 saves count events a minute apart into a database at
// path and leaves it as version 12 left it.
func storeAtVersion12(t *testing.T, path string, count int) []storage.ScoredEvent {
	t.Helper()
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	defer store.Close()
	saveVectorEvents(t, store, count)
	before := nearestHits(t, store)
	testcheck.NoError(t, rewriteStoredVectors(t, store, occurredAtRewrite))
	_, err = store.db.Exec(`PRAGMA user_version = 12`)
	testcheck.NoError(t, err)
	return before
}

func TestSpreadVectorDatesKeepsVectorsAndDatesEachOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	before := storeAtVersion12(t, path, 6)
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	defer store.Close()
	if after := nearestHits(t, store); !reflect.DeepEqual(before, after) || versionOf(t, store.db) != latestVersion() {
		t.Fatalf("expected the same hits after migration 13, got %+v then %+v", before, after)
	}
	var mismatched int
	testcheck.NoError(t, store.db.QueryRow(`SELECT COUNT(*) FROM chunk_embeddings JOIN chunks ON chunks.id = chunk_embeddings.chunk_id
		JOIN events ON events.id = chunks.event_id WHERE first_at != events.occurred_at OR last_at != events.occurred_at`).Scan(&mismatched))
	if mismatched != 0 {
		t.Fatalf("expected first_at and last_at to be each event's date, got %d mismatched", mismatched)
	}
}

func TestSearchSimilarFiltersByPeriodAfterMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	storeAtVersion12(t, path, 6)
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	defer store.Close()
	query := storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 6, From: baseTime.Add(2 * time.Minute), To: baseTime.Add(4 * time.Minute)}
	hits, err := store.SearchSimilar(context.Background(), query)
	testcheck.NoError(t, err)
	if len(hits) != 2 || hits[0].Event.UID != "e2" || hits[1].Event.UID != "e3" {
		t.Fatalf("expected e2 and e3, the events of minutes 2 and 3, got %+v", hits)
	}
}

// A table already in the layout (created by this version, then marked
// older, as other migration tests do) is left as it is.
func TestSpreadVectorDatesSkipsTheCurrentLayout(t *testing.T) {
	store := openTestStore(t)
	saveVectorEvents(t, store, 3)
	before := nearestHits(t, store)
	err := store.inTransaction(context.Background(), func(tx *sql.Tx) error { return spreadVectorDates(context.Background(), tx) })
	testcheck.NoError(t, err)
	if after := nearestHits(t, store); !reflect.DeepEqual(before, after) {
		t.Fatalf("expected the table untouched, got %+v then %+v", before, after)
	}
}

func TestVectorTableLacksReadsTheDeclaration(t *testing.T) {
	store := openTestStore(t)
	saveVectorEvents(t, store, 1)
	err := store.inTransaction(context.Background(), func(tx *sql.Tx) error {
		lacksDates, err := vectorTableLacks(context.Background(), tx, `first_at`)
		if err != nil || lacksDates {
			t.Errorf("expected first_at declared, got lacks=%v (err %v)", lacksDates, err)
		}
		lacksFloat, err := vectorTableLacks(context.Background(), tx, `float[`)
		if err != nil || !lacksFloat {
			t.Errorf("expected no float column, got lacks=%v (err %v)", lacksFloat, err)
		}
		return nil
	})
	testcheck.NoError(t, err)
}
