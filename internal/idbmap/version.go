package idbmap

import (
	"fmt"

	"github.com/chipskein/cade/internal/webstore"
)

// indexedDBOnlyVersion is the format of the schemas saved before records
// had a kind (#73): every one reads IndexedDB, and names its locations
// database_prefix and store.
const indexedDBOnlyVersion = 1

// upgrade reads a version 1 schema as the current format. The file keeps
// its old form on disk until the schema is replaced. Other versions are
// left for Validate to refuse.
func (s *Schema) upgrade() error {
	if s.Version == FormatVersion {
		return s.eachLocation((*Location).rejectV1)
	}
	if s.Version != indexedDBOnlyVersion {
		return nil
	}
	if s.Records.Kind != "" {
		return fmt.Errorf("records.kind %q in a version %d schema, expected version %d", s.Records.Kind, indexedDBOnlyVersion, FormatVersion)
	}
	if err := s.eachLocation((*Location).upgradeV1); err != nil {
		return err
	}
	s.Version, s.Records.Kind = FormatVersion, webstore.KindIndexedDB
	return nil
}

// eachLocation runs visit on the records, every lookup and every
// fingerprint shape, stopping at the first error.
func (s *Schema) eachLocation(visit func(*Location) error) error {
	locations := []*Location{&s.Records.Location}
	for _, rule := range s.Fields {
		if rule.Lookup != nil {
			locations = append(locations, &rule.Lookup.Location)
		}
	}
	for i := range s.Fingerprint {
		locations = append(locations, &s.Fingerprint[i].Location)
	}
	for _, location := range locations {
		if err := visit(location); err != nil {
			return err
		}
	}
	return nil
}
