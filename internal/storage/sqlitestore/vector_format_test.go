package sqlitestore

import (
	"database/sql"
	"fmt"
	"math"
	"slices"
	"testing"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"

	"github.com/chipskein/cade/internal/testcheck"
)

// Vector formats sqlite-vec can store, compared by space and by how much
// of the float32 top-k they keep (#40).

// vectorFormat is one way to store chunk vectors in a vec0 table.
type vectorFormat struct {
	name string
	// column is the vec0 column declaration; %d is the dimension count.
	column string
	// value turns the bound blob into the column's type.
	value  string
	encode func([]float32) []byte
	// bitsPerDimension is the stored size, to report bytes per vector.
	bitsPerDimension int
}

var (
	float32Format = vectorFormat{"float32", "float[%d] distance_metric=cosine", "?", float32Blob, 32}
	// int8Unit is sqlite-vec's own quantization: [-1, 1] onto [-128, 127].
	int8UnitFormat = vectorFormat{"int8 unit", "int8[%d] distance_metric=cosine", "vec_quantize_int8(?, 'unit')", float32Blob, 8}
	// int8Scaled scales each vector by its own largest component, which
	// cosine ignores; normalized 768-dim vectors rarely pass ±0.2, so the
	// unit scale leaves most of the 256 levels unused.
	int8ScaledFormat = vectorFormat{"int8 scaled", "int8[%d] distance_metric=cosine", "vec_int8(?)", int8ScaledBlob, 8}
	bitFormat        = vectorFormat{"bit", "bit[%d]", "vec_quantize_binary(?)", float32Blob, 1}
	vectorFormats    = []vectorFormat{float32Format, int8UnitFormat, int8ScaledFormat, bitFormat}
)

func (f vectorFormat) bytesPerVector(dimensions int) int {
	return dimensions * f.bitsPerDimension / 8
}

// float32Blob drops SerializeFloat32's error, which only a failing
// in-memory write could return.
func float32Blob(vector []float32) []byte {
	blob, _ := sqlitevec.SerializeFloat32(vector)
	return blob
}

func int8ScaledBlob(vector []float32) []byte {
	var largest float64
	for _, value := range vector {
		largest = math.Max(largest, math.Abs(float64(value)))
	}
	blob := make([]byte, len(vector))
	if largest == 0 {
		return blob
	}
	for i, value := range vector {
		blob[i] = byte(int8(math.Round(float64(value) * math.MaxInt8 / largest)))
	}
	return blob
}

// openFormatTable is an in-memory vec0 table "vectors" in format f; one
// connection, since each :memory: connection is its own database.
func openFormatTable(tb testing.TB, f vectorFormat, dimensions int) *sql.DB {
	tb.Helper()
	db, err := sql.Open(DriverName, ":memory:")
	testcheck.NoError(tb, err)
	tb.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(fmt.Sprintf(`CREATE VIRTUAL TABLE vectors USING vec0(id INTEGER PRIMARY KEY, embedding `+f.column+`)`, dimensions))
	testcheck.NoError(tb, err)
	return db
}

// fillFormatTable stores vectors[i] with id i.
func fillFormatTable(tb testing.TB, db *sql.DB, f vectorFormat, vectors [][]float32) {
	tb.Helper()
	tx, err := db.Begin()
	testcheck.NoError(tb, err)
	defer tx.Rollback()
	for id, vector := range vectors {
		_, err := tx.Exec(`INSERT INTO vectors (id, embedding) VALUES (?, `+f.value+`)`, id, f.encode(vector))
		testcheck.NoError(tb, err)
	}
	testcheck.NoError(tb, tx.Commit())
}

func nearestIDs(tb testing.TB, db *sql.DB, f vectorFormat, query []float32, k int) []int {
	tb.Helper()
	rows, err := db.Query(`SELECT id FROM vectors WHERE embedding MATCH `+f.value+` AND k = ? ORDER BY distance`, f.encode(query), k)
	testcheck.NoError(tb, err)
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		testcheck.NoError(tb, rows.Scan(&id))
		ids = append(ids, id)
	}
	testcheck.NoError(tb, rows.Err())
	return ids
}

// overlap is the share of want found in got: recall of the exact top-k.
func overlap(want, got []int) float64 {
	if len(want) == 0 {
		return 1
	}
	found := 0
	for _, id := range want {
		if slices.Contains(got, id) {
			found++
		}
	}
	return float64(found) / float64(len(want))
}

// rescoreByDot reorders candidate ids by the dot product of their float32
// vector with the query (cosine, for normalized vectors) and keeps k: the
// usual way to recover what binary quantization loses.
func rescoreByDot(candidates []int, corpus [][]float32, query []float32, k int) []int {
	ranked := slices.Clone(candidates)
	slices.SortStableFunc(ranked, func(a, b int) int {
		return -compareFloat(dot(corpus[a], query), dot(corpus[b], query))
	})
	return ranked[:min(k, len(ranked))]
}

func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestFloat32BlobIsLittleEndian(t *testing.T) {
	got := float32Blob([]float32{1, -2})
	want := []byte{0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0xc0}
	if !slices.Equal(got, want) {
		t.Errorf("float32Blob([1 -2]) = %x, want %x", got, want)
	}
}

func TestInt8ScaledBlobUsesTheWholeRange(t *testing.T) {
	got := int8ScaledBlob([]float32{0.1, -0.05, 0})
	want := []byte{127, 0xc0 /* int8(-64) */, 0}
	if !slices.Equal(got, want) {
		t.Errorf("int8ScaledBlob([0.1 -0.05 0]) = %v, want %v", got, want)
	}
	if zero := int8ScaledBlob([]float32{0, 0}); !slices.Equal(zero, []byte{0, 0}) {
		t.Errorf("int8ScaledBlob of a zero vector = %v, want zeros", zero)
	}
}

func TestOverlapIsRecallOfWant(t *testing.T) {
	if got := overlap([]int{1, 2, 3, 4}, []int{4, 9, 1}); got != 0.5 {
		t.Errorf("overlap = %v, want 0.5", got)
	}
	if got := overlap(nil, []int{1}); got != 1 {
		t.Errorf("overlap of an empty want = %v, want 1", got)
	}
}

func TestRescoreByDotKeepsTheClosest(t *testing.T) {
	corpus := [][]float32{{1, 0}, {0, 1}, {0.8, 0.6}}
	got := rescoreByDot([]int{0, 1, 2}, corpus, []float32{0.6, 0.8}, 2)
	if !slices.Equal(got, []int{2, 1}) {
		t.Errorf("rescoreByDot = %v, want [2 1]", got)
	}
}

func TestQuantizedFormatsFindTheVectorItself(t *testing.T) {
	corpus := [][]float32{{0.6, 0.8, 0, 0, 0, 0, 0, 0}, {0, 0, 0.6, -0.8, 0, 0, 0, 0}, {0, 0, 0, 0, 0, 0, -1, 0}}
	for _, f := range vectorFormats {
		db := openFormatTable(t, f, len(corpus[0]))
		fillFormatTable(t, db, f, corpus)
		if got := nearestIDs(t, db, f, corpus[1], 1); !slices.Equal(got, []int{1}) {
			t.Errorf("%s: nearest of vector 1 = %v, want [1]", f.name, got)
		}
	}
}
