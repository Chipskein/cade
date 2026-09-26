// Package benchmarks measures the local models as `cade` uses them: latency
// of embedding an event, interpreting a question and generating an answer,
// plus the memory they take. Storage benchmarks live next to the store
// (internal/storage/sqlitestore). Run with `make bench`.
package benchmarks
