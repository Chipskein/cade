package sqlitestore

import (
	"database/sql"
	"os"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

// realVectorsVar names a cade database (opened read-only) whose vectors
// TestQuantizationOverlap compares; unset, the test is skipped, since
// random vectors say nothing about how real embeddings quantize (#40).
const realVectorsVar = "CADE_SPACE_DB"

const (
	overlapK = 10
	// bitCandidates is how many binary neighbours are rescored in float32.
	bitCandidates = 10 * overlapK
	// corpusStride and queryStride sample chunk ids over the whole history
	// without loading every vector: a quarter as corpus, 1 in 1000 as
	// queries, never the same chunk in both.
	corpusStride = 4
	queryStride  = 1000
	queryOffset  = 1
)

// TestQuantizationOverlap reports, for each format, how much of the exact
// float32 top-10 it keeps on real vectors:
//
//	CADE_SPACE_DB=~/.local/share/cade/cade.db go test -tags sqlite_fts5 -run TestQuantizationOverlap -v ./internal/storage/sqlitestore
func TestQuantizationOverlap(t *testing.T) {
	path := os.Getenv(realVectorsVar)
	if path == "" {
		t.Skipf("%s is not set", realVectorsVar)
	}
	corpus := loadRealVectors(t, path, corpusStride, 0)
	queries := loadRealVectors(t, path, queryStride, queryOffset)
	exact := topKPerQuery(t, float32Format, corpus, queries, overlapK)
	t.Logf("%d corpus vectors, %d queries, top-%d", len(corpus), len(queries), overlapK)
	for _, f := range vectorFormats[1:] {
		found := topKPerQuery(t, f, corpus, queries, overlapK)
		t.Logf("%-12s %5d bytes/vector  top-%d overlap %.3f", f.name, f.bytesPerVector(len(corpus[0])), overlapK, meanOverlap(exact, found))
	}
	rescored := rescoreEach(topKPerQuery(t, bitFormat, corpus, queries, bitCandidates), corpus, queries)
	t.Logf("%-12s %5d bytes/vector  top-%d overlap %.3f (top-%d rescored in float32)", "bit+rescore", bitFormat.bytesPerVector(len(corpus[0])), overlapK, meanOverlap(exact, rescored), bitCandidates)
}

// loadRealVectors reads the vectors whose chunk_id % stride == offset.
func loadRealVectors(tb testing.TB, path string, stride, offset int) [][]float32 {
	tb.Helper()
	db, err := sql.Open(DriverName, "file:"+path+"?mode=ro")
	testcheck.NoError(tb, err)
	defer db.Close()
	rows, err := db.Query(`SELECT embedding FROM chunk_embeddings WHERE chunk_id % ? = ?`, stride, offset)
	testcheck.NoError(tb, err)
	defer rows.Close()
	var vectors [][]float32
	for rows.Next() {
		var blob []byte
		testcheck.NoError(tb, rows.Scan(&blob))
		vector, err := decodeFloat32s(blob)
		testcheck.NoError(tb, err)
		vectors = append(vectors, vector)
	}
	testcheck.NoError(tb, rows.Err())
	return vectors
}

func topKPerQuery(tb testing.TB, f vectorFormat, corpus, queries [][]float32, k int) [][]int {
	tb.Helper()
	db := openFormatTable(tb, f, len(corpus[0]))
	fillFormatTable(tb, db, f, corpus)
	found := make([][]int, len(queries))
	for i, query := range queries {
		found[i] = nearestIDs(tb, db, f, query, k)
	}
	return found
}

func rescoreEach(candidates [][]int, corpus, queries [][]float32) [][]int {
	rescored := make([][]int, len(queries))
	for i, query := range queries {
		rescored[i] = rescoreByDot(candidates[i], corpus, query, overlapK)
	}
	return rescored
}

func meanOverlap(exact, found [][]int) float64 {
	var sum float64
	for i := range exact {
		sum += overlap(exact[i], found[i])
	}
	return sum / float64(len(exact))
}

func TestMeanOverlapAveragesQueries(t *testing.T) {
	if got := meanOverlap([][]int{{1, 2}, {3, 4}}, [][]int{{1, 2}, {5, 6}}); got != 0.5 {
		t.Errorf("meanOverlap = %v, want 0.5", got)
	}
}
