package sqlitestore

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// Storage benchmarks run on synthetic stores shaped like a real history
// (108k events: 57% git, 31% browser, 12% teams; 768-dim vectors) so search
// latency and database growth can be tracked as the history grows.

const benchDimensions = 768

var (
	benchSizes = []int{1_000, 10_000, 100_000}
	benchNow   = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	benchWords = strings.Fields(`deploy cache redis login timeout pedido cliente relatório sincronização transportadora
		cep erro corrige ajusta refatora teste revisão reunião contrato tarefa banco query índice painel api
		autenticação token sessão página documentação configuração servidor homologação produção branch merge`)
	seeded sync.Map // size -> *benchStore
)

type benchStore struct {
	store *Store
	path  string
}

// benchSource mirrors the real mix of sources.
func benchSource(i int) event.Source {
	switch i % 100 {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11:
		return event.SourceTeams
	}
	if i%100 < 43 {
		return event.SourceBrowser
	}
	return event.SourceGit
}

func benchEvent(random *rand.Rand, i int) (event.Event, []float32) {
	words := make([]string, 20+random.Intn(20))
	for j := range words {
		words[j] = benchWords[random.Intn(len(benchWords))]
	}
	when := benchNow.Add(-time.Duration(random.Int63n(int64(365 * 24 * time.Hour))))
	ev := event.Event{UID: fmt.Sprintf("bench-%d", i), Source: benchSource(i), Timestamp: when, Content: strings.Join(words, " ")}
	if ev.Source == event.SourceTeams {
		ev.Metadata = event.Message{Sender: fmt.Sprintf("Pessoa %d", random.Intn(40)), Conversation: "chat"}.Metadata()
	}
	return ev, benchVector(random)
}

func benchVector(random *rand.Rand) []float32 {
	vector := make([]float32, benchDimensions)
	var norm float64
	for i := range vector {
		vector[i] = float32(random.NormFloat64())
		norm += float64(vector[i]) * float64(vector[i])
	}
	for i := range vector {
		vector[i] /= float32(math.Sqrt(norm))
	}
	return vector
}

// seededStore builds (once per process) a store with n events in a single
// transaction; per-event transactions would make 100k take minutes.
func seededStore(b *testing.B, n int) benchStore {
	b.Helper()
	if cached, ok := seeded.Load(n); ok {
		return cached.(benchStore)
	}
	path := filepath.Join(benchDirectory(b), fmt.Sprintf("bench-%d.db", n))
	store, err := Open(context.Background(), path)
	if err != nil {
		b.Fatal(err)
	}
	if err := seedEvents(store, n); err != nil {
		b.Fatal(err)
	}
	seeded.Store(n, benchStore{store: store, path: path})
	return benchStore{store: store, path: path}
}

// benchDirectory outlives each benchmark (b.TempDir does not), since the
// seeded stores are shared; TestMain removes it.
func benchDirectory(b *testing.B) string {
	benchDirOnce.Do(func() { benchDir, benchDirErr = os.MkdirTemp("", "cade-bench-*") })
	if benchDirErr != nil {
		b.Fatal(benchDirErr)
	}
	return benchDir
}

var (
	benchDirOnce sync.Once
	benchDir     string
	benchDirErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if benchDir != "" {
		os.RemoveAll(benchDir)
	}
	os.Exit(code)
}

func seedEvents(store *Store, n int) error {
	ctx := context.Background()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	random := rand.New(rand.NewSource(int64(n)))
	for i := range n {
		ev, vector := benchEvent(random, i)
		eventID, _, err := insertEventRow(ctx, tx, ev)
		if err != nil {
			return err
		}
		if err := insertChunks(ctx, tx, eventID, ev, []storage.Chunk{{End: len(ev.Content), Vector: vector}}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func forEachSize(b *testing.B, run func(b *testing.B, bench benchStore)) {
	for _, n := range benchSizes {
		b.Run(fmt.Sprintf("events=%d", n), func(b *testing.B) { run(b, seededStore(b, n)) })
	}
}

// BenchmarkSearchSimilar is the vector search of every unfiltered or
// source/period-filtered question.
func BenchmarkSearchSimilar(b *testing.B) {
	day := storage.SimilarityQuery{From: benchNow.AddDate(0, 0, -1), To: benchNow}
	filters := map[string]storage.SimilarityQuery{"all": {}, "source=teams": {Source: event.SourceTeams}, "one-day": day}
	forEachSize(b, func(b *testing.B, bench benchStore) {
		for name, filter := range filters {
			b.Run(name, func(b *testing.B) {
				query, random := filter, rand.New(rand.NewSource(1))
				query.Limit = 24
				for b.Loop() {
					query.Embedding = benchVector(random)
					if _, err := bench.store.SearchSimilar(context.Background(), query); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}

// BenchmarkEventsBetween reads a day (timeline, listings) and everything
// (a person question without a period loads the whole history).
func BenchmarkEventsBetween(b *testing.B) {
	spans := map[string]time.Duration{"one-day": 24 * time.Hour, "all": 400 * 24 * time.Hour}
	forEachSize(b, func(b *testing.B, bench benchStore) {
		for name, span := range spans {
			b.Run(name, func(b *testing.B) {
				for b.Loop() {
					if _, err := bench.store.EventsBetween(context.Background(), benchNow.Add(-span), benchNow); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}

// BenchmarkChunksFor loads the chunk vectors of a person's messages to rank
// them (the person-filtered answer path).
func BenchmarkChunksFor(b *testing.B) {
	forEachSize(b, func(b *testing.B, bench benchStore) {
		uids, size := make([]string, 0, 1000), bench.size(b)
		for i := 0; i < 1000 && i < size; i++ {
			uids = append(uids, fmt.Sprintf("bench-%d", i*size/1000))
		}
		for b.Loop() {
			if _, err := bench.store.ChunksFor(context.Background(), uids); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func (s benchStore) size(b *testing.B) int {
	var n int
	testcheck.NoError(b, s.store.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n))
	return n
}

// BenchmarkSaveEvent is one ingested event's write, in its own transaction
// as ingestion does, on a store that already holds 10k events.
func BenchmarkSaveEvent(b *testing.B) {
	bench := seededStore(b, 10_000)
	random := rand.New(rand.NewSource(2))
	i := 1_000_000
	for b.Loop() {
		ev, vector := benchEvent(random, i)
		if _, err := bench.store.SaveEvent(context.Background(), ev, whole(ev, vector)); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

// BenchmarkDatabaseSize reports bytes per event, to project growth.
func BenchmarkDatabaseSize(b *testing.B) {
	forEachSize(b, func(b *testing.B, bench benchStore) {
		for b.Loop() {
			if _, err := bench.store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
				b.Fatal(err)
			}
		}
		info, err := os.Stat(bench.path)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(info.Size())/float64(bench.size(b)), "bytes/event")
	})
}

// BenchmarkSearchLexical is the keyword half of hybrid retrieval: a word
// query and an identifier (hash prefix) query.
func BenchmarkSearchLexical(b *testing.B) {
	queries := map[string]string{"words": `"deploy" OR "cache" OR "login"`, "hash-prefix": `e5f6a7b*`}
	forEachSize(b, func(b *testing.B, bench benchStore) {
		for name, match := range queries {
			b.Run(name, func(b *testing.B) {
				for b.Loop() {
					if _, err := bench.store.SearchLexical(context.Background(), storage.LexicalQuery{Match: match, Limit: 96}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	})
}
