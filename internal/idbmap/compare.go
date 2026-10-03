package idbmap

import "github.com/chipskein/cade/internal/indexeddb"

// minUIDOverlap is the share of the current schema's events a replacement
// must still produce under the same UID. Below it, messages already indexed
// would come back as new events under other UIDs, duplicated.
const minUIDOverlap = 0.9

// Comparison measures a replacement against the current schema on the
// same records.
type Comparison struct {
	CurrentEvents int
	NextEvents    int
	SharedUIDs    int
}

// CompareSchemas applies both schemas to records.
//
//	comparison, err := idbmap.CompareSchemas(current, regenerated, records)
func CompareSchemas(current, next Schema, records []indexeddb.Record) (Comparison, error) {
	currentUIDs, err := eventUIDs(current, records)
	if err != nil {
		return Comparison{}, err
	}
	nextUIDs, err := eventUIDs(next, records)
	if err != nil {
		return Comparison{}, err
	}
	comparison := Comparison{CurrentEvents: len(currentUIDs), NextEvents: len(nextUIDs)}
	for uid := range nextUIDs {
		if currentUIDs[uid] {
			comparison.SharedUIDs++
		}
	}
	return comparison, nil
}

// Overlap is the share of the current schema's events the next one keeps;
// a current schema that maps nothing (the application changed) has nothing
// to keep.
func (c Comparison) Overlap() float64 {
	if c.CurrentEvents == 0 {
		return 1
	}
	return float64(c.SharedUIDs) / float64(c.CurrentEvents)
}

// Accepted reports whether next may replace the current schema: it maps
// something, at least as much, and keeps the UIDs of what is indexed.
func (c Comparison) Accepted() bool {
	return c.NextEvents > 0 && c.NextEvents >= c.CurrentEvents && c.Overlap() >= minUIDOverlap
}

func eventUIDs(schema Schema, records []indexeddb.Record) (map[string]bool, error) {
	mapper, err := NewMapper(schema)
	if err != nil {
		return nil, err
	}
	events, _, err := mapper.Apply(records)
	if err != nil {
		return nil, err
	}
	uids := make(map[string]bool, len(events))
	for _, mapped := range events {
		uids[mapped.UID] = true
	}
	return uids, nil
}
