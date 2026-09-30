//go:build sqlite_dbstat

package sqlitestore

import (
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

// pagesPerTable sums dbstat's page bytes by the table each b-tree belongs
// to; joining sqlite_schema puts every index under its table.
const pagesPerTable = `
SELECT s.tbl_name, sum(d.pgsize)
FROM dbstat d JOIN sqlite_schema s ON s.name = d.name
GROUP BY s.tbl_name`

// BenchmarkTableSize reports bytes per event of each table with its
// indexes (#40), to know where the space of BenchmarkDatabaseSize goes.
// The synthetic history has no files, so file_modifications stays at 0.
func BenchmarkTableSize(b *testing.B) {
	forEachSize(b, func(b *testing.B, bench benchStore) {
		for b.Loop() {
			_, err := bench.store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
			testcheck.NoError(b, err)
		}
		events := float64(bench.size(b))
		for table, bytes := range bench.bytesPerTable(b) {
			b.ReportMetric(float64(bytes)/events, table+"-bytes/event")
		}
	})
}

func (s benchStore) bytesPerTable(b *testing.B) map[string]int64 {
	b.Helper()
	rows, err := s.store.db.Query(pagesPerTable)
	testcheck.NoError(b, err)
	defer rows.Close()
	sizes := make(map[string]int64, len(spaceTables))
	for _, table := range spaceTables {
		sizes[table] = 0
	}
	for rows.Next() {
		var tableName string
		var bytes int64
		testcheck.NoError(b, rows.Scan(&tableName, &bytes))
		sizes[spaceTableOf(tableName)] += bytes
	}
	testcheck.NoError(b, rows.Err())
	return sizes
}
