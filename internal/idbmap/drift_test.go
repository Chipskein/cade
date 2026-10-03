package idbmap

import (
	"slices"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
)

func chainSchemaWithFingerprint(t *testing.T) Schema {
	t.Helper()
	schema := mustMapper(t, chainSchema).schema
	schema.Fingerprint = TakeFingerprint(schema, chainRecords())
	return schema
}

func shapeOf(shapes []PathShape, store string, path Path) (PathShape, bool) {
	index := slices.IndexFunc(shapes, func(shape PathShape) bool { return shape.Store == store && shape.Path == path })
	if index < 0 {
		return PathShape{}, false
	}
	return shapes[index], true
}

func TestTakeFingerprintRecordsWhatTheSchemaReads(t *testing.T) {
	shapes := chainSchemaWithFingerprint(t).Fingerprint
	cases := map[[2]string][]string{
		{"chains", "$.messageMap.<id>"}:                     {"object"},
		{"chains", "$.messageMap.<id>.originalArrivalTime"}: {"number"},
		{"chains", "$.messageMap.<id>.deletionInfo"}:        {"object"},
		{"profiles", "$.displayName"}:                       {"string"},
	}
	for key, kinds := range cases {
		shape, found := shapeOf(shapes, key[0], Path(key[1]))
		if !found || !slices.Equal(shape.Kinds, kinds) {
			t.Errorf("%v: expected kinds %v, got %+v (found %v)", key, kinds, shape, found)
		}
	}
	if _, found := shapeOf(shapes, "chains", "$.messageMap.<id>.imDisplayName"); !found {
		t.Error("expected every field path in the fingerprint")
	}
}

func TestMeasureDriftOnTheSameRecords(t *testing.T) {
	drift, err := MeasureDrift(chainSchemaWithFingerprint(t), chainRecords())
	if err != nil || drift.Drifted() || drift.Tally.Mapped != 2 {
		t.Fatalf("expected no drift, got %+v (err %v)", drift, err)
	}
}

// renamedRecords is the same chat after the app renamed content to body
// and started writing the arrival time as text.
func renamedRecords() []indexeddb.Record {
	records := chainRecords()
	for _, message := range records[0].Value.Get("messageMap").Properties {
		for i := range message.Value.Properties {
			property := &message.Value.Properties[i]
			switch property.Key {
			case "content":
				property.Key = "body"
			case "originalArrivalTime":
				property.Value = str("2026-09-25T15:00:00Z")
			}
		}
	}
	return records
}

func TestMeasureDriftFindsMissingAndChangedPaths(t *testing.T) {
	drift, err := MeasureDrift(chainSchemaWithFingerprint(t), renamedRecords())
	if err != nil || !drift.Drifted() {
		t.Fatalf("expected drift, got %+v (err %v)", drift, err)
	}
	if _, missing := shapeOf(drift.Missing, "chains", "$.messageMap.<id>.content"); !missing {
		t.Errorf("expected content missing, got %+v", drift.Missing)
	}
	changed, found := shapeOf(drift.Changed, "chains", "$.messageMap.<id>.originalArrivalTime")
	if !found || !slices.Equal(changed.Kinds, []string{"string"}) {
		t.Errorf("expected the arrival time now a string, got %+v", drift.Changed)
	}
}

func TestMeasureDriftReportsAGoneStoreEvenWithoutFingerprint(t *testing.T) {
	records := chainRecords()
	for i := range records {
		records[i].Container = strings.ToUpper(records[i].Container)
	}
	drift, err := MeasureDrift(mustMapper(t, chainSchema).schema, records)
	if err != nil || drift.Format == nil || !drift.Drifted() || len(drift.Missing) != 0 {
		t.Fatalf("expected only the format error, got %+v (err %v)", drift, err)
	}
}
