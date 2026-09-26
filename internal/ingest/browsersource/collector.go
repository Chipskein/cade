// Package browsersource ingests page visits from a browser's local SQLite
// history (RF1.2). Firefox (places.sqlite) and Chromium-based browsers
// (History) are detected by schema.
package browsersource

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
)

// OpenDatabase opens a SQLite file; injected so this package does not pick
// the driver.
type OpenDatabase func(path string) (*sql.DB, error)

// Collector reads the visit history from one browser profile database.
type Collector struct {
	open        OpenDatabase
	historyPath string
}

// NewCollector reads historyPath (places.sqlite or History) using open.
//
//	collector := browsersource.NewCollector(openSQLite, "~/.mozilla/firefox/abc.default/places.sqlite")
func NewCollector(open OpenDatabase, historyPath string) *Collector {
	return &Collector{open: open, historyPath: historyPath}
}

// CollectEvents emits one event per visit (not per URL), so revisits show
// up in the timeline at every time they happened.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	historyPath, err := filepath.Abs(c.historyPath)
	if err != nil {
		return fmt.Errorf("resolve history path %q: %w", c.historyPath, err)
	}
	tempDir, snapshot, err := snapshotHistory(historyPath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	db, err := c.open(snapshot)
	if err != nil {
		return fmt.Errorf("open history snapshot of %q: %w", historyPath, err)
	}
	defer db.Close()
	return emitVisits(ctx, db, historyPath, emit)
}

func emitVisits(ctx context.Context, db *sql.DB, historyPath string, emit ingest.EmitFunc) error {
	flavor, err := detectFlavor(ctx, db)
	if err != nil {
		return fmt.Errorf("detect browser of %q: %w", historyPath, err)
	}
	rows, err := db.QueryContext(ctx, flavor.visitsQuery)
	if err != nil {
		return fmt.Errorf("query %s visits in %q: %w", flavor.name, historyPath, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := emitVisit(rows, flavor, historyPath, emit); err != nil {
			return err
		}
	}
	return rows.Err()
}

func emitVisit(rows *sql.Rows, flavor historyFlavor, historyPath string, emit ingest.EmitFunc) error {
	var rawTime int64
	var url, title string
	if err := rows.Scan(&rawTime, &url, &title); err != nil {
		return fmt.Errorf("scan %s visit row: %w", flavor.name, err)
	}
	return emit(visitEvent(flavor.name, historyPath, flavor.toTime(rawTime), url, title))
}

// visitEvent keys deduplication on browser + URL + microsecond visit time:
// independent of the file path (so a moved profile does not re-ingest) yet
// distinct for every real visit (RNF3.2).
func visitEvent(browser, historyPath string, visitedAt time.Time, url, title string) event.Event {
	content := url
	if title != "" {
		content = title + "\n" + url
	}
	return event.Event{
		UID:       event.StableID(event.SourceBrowser, browser, url, strconv.FormatInt(visitedAt.UnixMicro(), 10)),
		Timestamp: visitedAt,
		Source:    event.SourceBrowser,
		Content:   content,
		Metadata:  event.Metadata{"browser": browser, "url": url, "title": title, "history": historyPath},
	}
}
