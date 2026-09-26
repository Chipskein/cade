package browsersource

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Chromium stores visit times as microseconds since 1601-01-01 UTC.
const chromiumEpochOffsetMicros = 11_644_473_600_000_000

// historyFlavor knows one browser's schema. Every query returns
// (raw visit time, url, title).
type historyFlavor struct {
	name        string
	marker      string
	visitsQuery string
	toTime      func(raw int64) time.Time
}

var firefoxFlavor = historyFlavor{
	name:   "firefox",
	marker: "moz_historyvisits",
	visitsQuery: `SELECT v.visit_date, p.url, COALESCE(p.title, '')
		FROM moz_historyvisits v JOIN moz_places p ON p.id = v.place_id
		ORDER BY v.visit_date, v.id`,
	toTime: func(raw int64) time.Time { return time.UnixMicro(raw).UTC() },
}

var chromiumFlavor = historyFlavor{
	name:   "chromium",
	marker: "visits",
	visitsQuery: `SELECT v.visit_time, u.url, COALESCE(u.title, '')
		FROM visits v JOIN urls u ON u.id = v.url
		ORDER BY v.visit_time, v.id`,
	toTime: func(raw int64) time.Time { return time.UnixMicro(raw - chromiumEpochOffsetMicros).UTC() },
}

var knownFlavors = []historyFlavor{firefoxFlavor, chromiumFlavor}

func detectFlavor(ctx context.Context, db *sql.DB) (historyFlavor, error) {
	for _, flavor := range knownFlavors {
		found, err := tableExists(ctx, db, flavor.marker)
		if err != nil || found {
			return flavor, err
		}
	}
	return historyFlavor{}, fmt.Errorf("no moz_historyvisits or visits table; expected a Firefox places.sqlite or Chromium History file")
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, table).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("look up table %q: %w", table, err)
	}
	return exists, nil
}
