package testfakes

import (
	"strings"

	"github.com/chipskein/cade/internal/webstore"
)

// FakeStoreReader is a webstore.Reader of any kind: it recognizes the
// locations ending in Suffix and returns Records (or Err) for them,
// remembering what it read.
type FakeStoreReader struct {
	StoreKind     webstore.Kind
	Suffix        string
	Records       []webstore.Record
	Err           error
	ReadLocations []string
}

func (f *FakeStoreReader) Kind() webstore.Kind { return f.StoreKind }

func (f *FakeStoreReader) Recognizes(location string) bool {
	return strings.HasSuffix(location, f.Suffix)
}

func (f *FakeStoreReader) Read(location string) ([]webstore.Record, error) {
	f.ReadLocations = append(f.ReadLocations, location)
	return f.Records, f.Err
}
