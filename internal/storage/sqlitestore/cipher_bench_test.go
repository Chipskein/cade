package sqlitestore

import (
	"context"
	"math/rand"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/chipskein/cade/internal/storage"
)

// cipherBenchEvents is the history size of the acceptance of #111.
const cipherBenchEvents = 100_000

// sqliteDefaultCacheKiB is SQLite's own page cache, 2 MB, which #57
// showed too small once every page read has to be decrypted.
const sqliteDefaultCacheKiB = 2000

// storeOpener opens one variant of the benchmark database.
type storeOpener func(b *testing.B) *Store

// BenchmarkEncryptedSearch measures what encrypting the database (#111)
// costs the two searches of every `ask`: warm repeats the query on one
// connection; cold opens a connection per query, as each `cade ask` is a
// new process whose SQLite cache starts empty (the system's file cache
// stays warm). open is the cost of opening, key derivation included.
func BenchmarkEncryptedSearch(b *testing.B) {
	bench := seededStore(b, cipherBenchEvents)
	for variant, open := range cipherBenchVariants(b, bench) {
		b.Run(variant+"/open", func(b *testing.B) {
			for range b.N {
				benchClose(b, open(b))
			}
		})
		for query, search := range cipherBenchQueries() {
			b.Run(variant+"/"+query+"/warm", func(b *testing.B) { benchWarm(b, open, search) })
			b.Run(variant+"/"+query+"/cold", func(b *testing.B) { benchCold(b, open, search) })
		}
	}
}

// cipherBenchVariants are the plain database, the encrypted one with the
// cache a keyed open sets, and the encrypted one with SQLite's default.
func cipherBenchVariants(b *testing.B, bench benchStore) map[string]storeOpener {
	encrypted := filepath.Join(filepath.Dir(bench.path), "bench-encrypted.db")
	if err := copyWithKey(context.Background(), bench.path, nil, encrypted, testCipherKey); err != nil {
		b.Fatal(err)
	}
	openEncrypted := func(b *testing.B) *Store { return benchOpen(b, encrypted, testCipherKey) }
	return map[string]storeOpener{
		"plain":     func(b *testing.B) *Store { return benchOpen(b, bench.path, nil) },
		"encrypted": openEncrypted,
		"encrypted,cache=2MB": func(b *testing.B) *Store {
			store := openEncrypted(b)
			if _, err := store.db.Exec(`PRAGMA cache_size = -` + strconv.Itoa(sqliteDefaultCacheKiB)); err != nil {
				b.Fatal(err)
			}
			return store
		},
	}
}

func benchOpen(b *testing.B, path string, key CipherKey) *Store {
	store, err := OpenWithKey(context.Background(), path, key, Hooks{})
	if err != nil {
		b.Fatal(err)
	}
	return store
}

func benchClose(b *testing.B, store *Store) {
	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
}

// cipherBenchQueries are an unfiltered vector search and a search for
// common words, as in BenchmarkSearchSimilar and BenchmarkSearchLexical.
func cipherBenchQueries() map[string]func(*Store, *rand.Rand) error {
	return map[string]func(*Store, *rand.Rand) error{
		"similar": func(store *Store, random *rand.Rand) error {
			_, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: benchVector(random), Limit: 24})
			return err
		},
		"lexical": func(store *Store, _ *rand.Rand) error {
			_, err := store.SearchLexical(context.Background(), storage.LexicalQuery{Match: `"deploy" OR "cache" OR "login"`, Limit: 96})
			return err
		},
	}
}

func benchWarm(b *testing.B, open storeOpener, search func(*Store, *rand.Rand) error) {
	store, random := open(b), rand.New(rand.NewSource(1))
	defer benchClose(b, store)
	for b.Loop() {
		if err := search(store, random); err != nil {
			b.Fatal(err)
		}
	}
}

// benchCold times only the first query of each fresh connection.
func benchCold(b *testing.B, open storeOpener, search func(*Store, *rand.Rand) error) {
	random := rand.New(rand.NewSource(1))
	for range b.N {
		b.StopTimer()
		store := open(b)
		b.StartTimer()
		if err := search(store, random); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		benchClose(b, store)
		b.StartTimer()
	}
}
