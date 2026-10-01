package sqlitestore

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testcheck"
)

// Where the source and period filters go once a text's chunks are shared
// by every event with that text (#67). A text never spans two sources in
// the real history, so source stays a vec0 metadata column; its events can
// be years apart, so the period cannot. Three designs, each returning the
// k nearest texts with an event in the source and period, like the scan:
//   - afterKNN: the KNN ignores the period; the events check it, and k
//     widens until k texts pass.
//   - dateRange: vec0 keeps the text's first and last date; the KNN keeps
//     the texts whose range overlaps the period, the events check the rest.
//   - chunkIDs: the KNN gets `text_id IN (texts with an event in the
//     period)`, which sqlite-vec 0.1.6 applies inside the scan.

// Repetition measured on the reference machine's history, 2026-10-01: 9%
// of the texts repeat, and a repeated text's events are 0.4 days apart at
// the median, 133 at p90, 613 at p99.
const (
	repeatedTextShare = 0.09
	repeatTailIndex   = 1.2
	maxRepeats        = 2000
	historySpan       = 2 * 365 * 24 * time.Hour
)

var repeatSpreads = []struct {
	upTo     float64
	from, to time.Duration
}{
	{0.5, 0, 24 * time.Hour},
	{0.9, 24 * time.Hour, 133 * 24 * time.Hour},
	{1, 133 * 24 * time.Hour, 613 * 24 * time.Hour},
}

// sharedText is one distinct text: its source, its events' times (unix
// milliseconds) and its vector.
type sharedText struct {
	source event.Source
	times  []int64
	vector []float32
}

func sharedHistory(texts, dimensions int, random *rand.Rand) []sharedText {
	history := make([]sharedText, texts)
	for i := range history {
		vector := benchVector(random)[:dimensions]
		history[i] = sharedText{source: benchSource(i), times: eventTimes(random), vector: vector}
	}
	return history
}

// eventTimes spreads a text's events over a window ending at benchNow.
func eventTimes(random *rand.Rand) []int64 {
	count, spread := repeatCount(random), repeatSpread(random)
	start := benchNow.Add(-time.Duration(random.Int63n(int64(historySpan-spread+1))) - spread)
	times := make([]int64, count)
	for i := range times {
		times[i] = toUnixMillis(start.Add(time.Duration(random.Int63n(int64(spread) + 1))))
	}
	return times
}

// repeatCount is 1 for most texts and a Pareto tail for the rest: the
// real history has one text with 7,297 events.
func repeatCount(random *rand.Rand) int {
	if random.Float64() >= repeatedTextShare {
		return 1
	}
	return min(int(2/math.Pow(1-random.Float64(), 1/repeatTailIndex)), maxRepeats)
}

func repeatSpread(random *rand.Rand) time.Duration {
	u := random.Float64()
	for _, spread := range repeatSpreads {
		if u < spread.upTo {
			return spread.from + time.Duration(random.Int63n(int64(spread.to-spread.from)+1))
		}
	}
	return 0
}

const createSharedChunkLayout = `
CREATE TABLE shared_events (id INTEGER PRIMARY KEY, text_id INTEGER NOT NULL, occurred_at INTEGER NOT NULL);
CREATE INDEX shared_events_text ON shared_events (text_id, occurred_at);
CREATE INDEX shared_events_time ON shared_events (occurred_at, text_id);
CREATE VIRTUAL TABLE shared_vectors USING vec0(
	text_id INTEGER PRIMARY KEY, embedding int8[%[1]d] distance_metric=cosine,
	source TEXT, first_at INTEGER, last_at INTEGER);
CREATE VIRTUAL TABLE event_vectors USING vec0(
	event_id INTEGER PRIMARY KEY, embedding int8[%[1]d] distance_metric=cosine,
	source TEXT, occurred_at INTEGER);`

// openSharedChunkLayout stores history both ways: one vector per text
// (shared_vectors) and, as today, one per event (event_vectors).
func openSharedChunkLayout(tb testing.TB, history []sharedText) *sql.DB {
	tb.Helper()
	db, err := sql.Open(DriverName, ":memory:")
	testcheck.NoError(tb, err)
	tb.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(fmt.Sprintf(createSharedChunkLayout, len(history[0].vector)))
	testcheck.NoError(tb, err)
	tx, err := db.Begin()
	testcheck.NoError(tb, err)
	defer tx.Rollback()
	for id, text := range history {
		testcheck.NoError(tb, insertSharedText(tx, int64(id), text))
	}
	testcheck.NoError(tb, tx.Commit())
	return db
}

func insertSharedText(tx *sql.Tx, id int64, text sharedText) error {
	blob := encodeInt8Vector(text.vector)
	_, err := tx.Exec(`INSERT INTO shared_vectors (text_id, embedding, source, first_at, last_at) VALUES (?, `+int8VectorValue+`, ?, ?, ?)`,
		id, blob, string(text.source), slices.Min(text.times), slices.Max(text.times))
	for _, at := range text.times {
		if err != nil {
			return err
		}
		err = insertSharedEvent(tx, id, blob, text.source, at)
	}
	return err
}

// insertSharedEvent stores one event of text id and, as today, its own
// copy of the vector.
func insertSharedEvent(tx *sql.Tx, id int64, blob []byte, source event.Source, at int64) error {
	result, err := tx.Exec(`INSERT INTO shared_events (text_id, occurred_at) VALUES (?, ?)`, id, at)
	if err != nil {
		return err
	}
	eventID, _ := result.LastInsertId()
	_, err = tx.Exec(`INSERT INTO event_vectors (event_id, embedding, source, occurred_at) VALUES (?, `+int8VectorValue+`, ?, ?)`,
		eventID, blob, string(source), at)
	return err
}

// sharedChunkQuery is a filtered search for k texts; source is "" for all.
type sharedChunkQuery struct {
	embedding []float32
	k         int
	source    event.Source
	from, to  int64
}

func (q sharedChunkQuery) sourceArgs() (string, string) {
	return sourceFilter(q.source)
}

// sharedChunkDesign finds the query's k nearest texts, closest first.
type sharedChunkDesign struct {
	name   string
	search func(db *sql.DB, query sharedChunkQuery) ([]int64, error)
}

var sharedChunkDesigns = []sharedChunkDesign{
	{"afterKNN", searchAfterKNN},
	{"dateRange", searchDateRange},
	{"chunkIDs", searchChunkIDs},
}

// inPeriod is 1 when text_id has an event in [from, to).
const inPeriod = `EXISTS (SELECT 1 FROM shared_events WHERE shared_events.text_id = nearest.text_id AND occurred_at >= ? AND occurred_at < ?)`

func searchAfterKNN(db *sql.DB, query sharedChunkQuery) ([]int64, error) {
	operator, source := query.sourceArgs()
	statement := fmt.Sprintf(`WITH nearest AS (SELECT text_id, distance FROM shared_vectors
		WHERE embedding MATCH `+int8VectorValue+` AND k = ? AND source %s ?)
		SELECT text_id, `+inPeriod+` FROM nearest ORDER BY distance`, operator)
	return widenUntilK(query, func(limit int) (*sql.Rows, error) {
		return db.Query(statement, encodeInt8Vector(query.embedding), limit, source, query.from, query.to)
	})
}

func searchDateRange(db *sql.DB, query sharedChunkQuery) ([]int64, error) {
	operator, source := query.sourceArgs()
	statement := fmt.Sprintf(`WITH nearest AS (SELECT text_id, distance FROM shared_vectors
		WHERE embedding MATCH `+int8VectorValue+` AND k = ? AND source %s ? AND first_at < ? AND last_at >= ?)
		SELECT text_id, `+inPeriod+` FROM nearest ORDER BY distance`, operator)
	return widenUntilK(query, func(limit int) (*sql.Rows, error) {
		return db.Query(statement, encodeInt8Vector(query.embedding), limit, source, query.to, query.from, query.from, query.to)
	})
}

// widenUntilK runs the KNN with k, 4k, 16k... until k of the neighbours
// pass the period, the table has no more, or k reaches maxNeighbours. Each
// row is a text id and whether it passed.
func widenUntilK(query sharedChunkQuery, nearest func(limit int) (*sql.Rows, error)) ([]int64, error) {
	for limit := query.k; ; limit = min(limit*4, maxNeighbours) {
		rows, err := nearest(limit)
		if err != nil {
			return nil, err
		}
		passed, seen, err := scanPassedTexts(rows)
		if err != nil || len(passed) >= query.k || seen < limit || limit == maxNeighbours {
			return passed[:min(len(passed), query.k)], err
		}
	}
}

func scanPassedTexts(rows *sql.Rows) ([]int64, int, error) {
	defer rows.Close()
	var passed []int64
	seen := 0
	for ; rows.Next(); seen++ {
		var id int64
		var inPeriod bool
		if err := rows.Scan(&id, &inPeriod); err != nil {
			return nil, 0, err
		}
		if inPeriod {
			passed = append(passed, id)
		}
	}
	return passed, seen, rows.Err()
}

func searchChunkIDs(db *sql.DB, query sharedChunkQuery) ([]int64, error) {
	operator, source := query.sourceArgs()
	rows, err := db.Query(fmt.Sprintf(`SELECT text_id FROM shared_vectors
		WHERE embedding MATCH `+int8VectorValue+` AND k = ? AND source %s ?
		AND text_id IN (SELECT text_id FROM shared_events WHERE occurred_at >= ? AND occurred_at < ?)
		ORDER BY distance`, operator), encodeInt8Vector(query.embedding), query.k, source, query.from, query.to)
	if err != nil {
		return nil, err
	}
	return collectInt64s(rows)
}

func collectInt64s(rows *sql.Rows) ([]int64, error) {
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// searchPerEvent is today's search, for the latency it is compared with:
// one vector per event, both filters inside the KNN, k events (copies of
// one text take several).
func searchPerEvent(db *sql.DB, query sharedChunkQuery) ([]int64, error) {
	operator, source := query.sourceArgs()
	rows, err := db.Query(fmt.Sprintf(`SELECT event_id FROM event_vectors
		WHERE embedding MATCH `+int8VectorValue+` AND k = ? AND source %s ? AND occurred_at >= ? AND occurred_at < ?
		ORDER BY distance`, operator), encodeInt8Vector(query.embedding), query.k, source, query.from, query.to)
	if err != nil {
		return nil, err
	}
	return collectInt64s(rows)
}

// scanNearestTexts is the exact answer: every text in the source with an
// event in the period, by cosine distance of its stored (int8) vector.
func scanNearestTexts(history []sharedText, query sharedChunkQuery) []int64 {
	type scored struct {
		id       int64
		distance float64
	}
	var matching []scored
	unit := decodeInt8Vector(encodeInt8Vector(query.embedding))
	for id, text := range history {
		if textMatches(text, query) {
			matching = append(matching, scored{int64(id), 1 - dot(unit, decodeInt8Vector(encodeInt8Vector(text.vector)))})
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].distance < matching[j].distance })
	ids := make([]int64, 0, query.k)
	for _, text := range matching[:min(len(matching), query.k)] {
		ids = append(ids, text.id)
	}
	return ids
}

func textMatches(text sharedText, query sharedChunkQuery) bool {
	if query.source != "" && text.source != query.source {
		return false
	}
	return slices.ContainsFunc(text.times, func(at int64) bool { return at >= query.from && at < query.to })
}

// sharedChunkFilters are the filters retrieval sends, over the synthetic
// history's two years.
func sharedChunkFilters() map[string]sharedChunkQuery {
	since := func(span time.Duration) sharedChunkQuery {
		return sharedChunkQuery{from: toUnixMillis(benchNow.Add(-span)), to: toUnixMillis(benchNow)}
	}
	all := sharedChunkQuery{from: math.MinInt64, to: math.MaxInt64}
	teams := all
	teams.source = event.SourceTeams
	return map[string]sharedChunkQuery{
		"all": all, "source=teams": teams,
		"one-day": since(24 * time.Hour), "one-month": since(30 * 24 * time.Hour), "one-year": since(365 * 24 * time.Hour),
	}
}

// TestSharedChunkDesignsMatchTheScan: each design returns the exact k
// nearest texts of the source and period (CA9.1), on a history small
// enough to scan.
func TestSharedChunkDesignsMatchTheScan(t *testing.T) {
	random := rand.New(rand.NewSource(67))
	history := sharedHistory(3_000, 16, random)
	db := openSharedChunkLayout(t, history)
	for name, filter := range sharedChunkFilters() {
		for round := range 5 {
			query := filter
			query.k, query.embedding = 24, benchVector(random)[:16]
			want := scanNearestTexts(history, query)
			for _, design := range sharedChunkDesigns {
				got, err := design.search(db, query)
				testcheck.NoError(t, err)
				if !slices.Equal(got, want) {
					t.Errorf("%s, %s, round %d: got %v, want %v", design.name, name, round, got, want)
				}
			}
		}
	}
}

func TestEventTimesStayInTheHistory(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	oldest := toUnixMillis(benchNow.Add(-historySpan))
	for range 1_000 {
		for _, at := range eventTimes(random) {
			if at < oldest || at > toUnixMillis(benchNow) {
				t.Fatalf("event time %d outside [%d, %d]", at, oldest, toUnixMillis(benchNow))
			}
		}
	}
}

func TestRepeatCountIsAtLeastOne(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	repeated := 0
	for range 10_000 {
		count := repeatCount(random)
		if count < 1 || count > maxRepeats {
			t.Fatalf("repeatCount = %d, want 1..%d", count, maxRepeats)
		}
		if count > 1 {
			repeated++
		}
	}
	if share := float64(repeated) / 10_000; math.Abs(share-repeatedTextShare) > 0.01 {
		t.Errorf("repeated share = %.3f, want about %.2f", share, repeatedTextShare)
	}
}

// BenchmarkSharedChunkFilters times each design (and today's per-event
// search) for every filter, at 768 dimensions; hits/op is how many ids it
// returned: texts for the designs, events (copies included) for perEvent.
func BenchmarkSharedChunkFilters(b *testing.B) {
	for _, texts := range []int{5_000, 50_000} {
		history := sharedHistory(texts, benchDimensions, rand.New(rand.NewSource(int64(texts))))
		db := openSharedChunkLayout(b, history)
		designs := append(slices.Clone(sharedChunkDesigns), sharedChunkDesign{"perEvent", searchPerEvent})
		for name, filter := range sharedChunkFilters() {
			for _, design := range designs {
				b.Run(fmt.Sprintf("texts=%d/%s/%s", texts, name, design.name), func(b *testing.B) {
					runSharedChunkSearch(b, db, design, filter)
				})
			}
		}
	}
}

func runSharedChunkSearch(b *testing.B, db *sql.DB, design sharedChunkDesign, filter sharedChunkQuery) {
	random := rand.New(rand.NewSource(1))
	found := 0
	for b.Loop() {
		query := filter
		query.k, query.embedding = 24, benchVector(random)
		ids, err := design.search(db, query)
		testcheck.NoError(b, err)
		found += len(ids)
	}
	b.ReportMetric(float64(found)/float64(b.N), "hits/op")
}
