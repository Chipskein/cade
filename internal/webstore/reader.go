package webstore

import (
	"fmt"
	"slices"
)

// Reader reads one kind of storage of one origin from where a browser
// profile keeps it. A new storage enters cade by implementing Reader and
// joining Readers; the collector and the discovery stay as they are.
type Reader interface {
	Kind() Kind
	// Recognizes reports whether location has this reader's layout,
	// without reading the records.
	Recognizes(location string) bool
	Read(location string) ([]Record, error)
}

// Readers are the readers of a build, Chromium's and Firefox's IndexedDB
// among them: one kind may have a reader per browser.
type Readers []Reader

// Detect finds the reader of location by its layout: discovery is pointed
// at a directory and learns from it which storage the schema reads.
//
//	reader, err := readers.Detect("~/.floorp/x.default/storage/default/https+++web.whatsapp.com/idb")
func (rs Readers) Detect(location string) (Reader, error) {
	for _, reader := range rs {
		if reader.Recognizes(location) {
			return reader, nil
		}
	}
	return nil, fmt.Errorf("no reader recognizes %q, expected the layout of one of %s", location, joinKinds(rs.kinds()))
}

// ReadKind reads location with a reader of kind that recognizes it: a
// schema names its kind, and the location must still have that layout.
//
//	records, err := readers.ReadKind(webstore.KindIndexedDB, dir)
func (rs Readers) ReadKind(kind Kind, location string) ([]Record, error) {
	for _, reader := range rs {
		if reader.Kind() == kind && reader.Recognizes(location) {
			return reader.Read(location)
		}
	}
	return nil, fmt.Errorf("no %s reader recognizes %q, expected a location with the layout of that storage", kind, location)
}

// kinds lists the kinds of rs once each, in order.
func (rs Readers) kinds() []Kind {
	var kinds []Kind
	for _, reader := range rs {
		if !slices.Contains(kinds, reader.Kind()) {
			kinds = append(kinds, reader.Kind())
		}
	}
	return kinds
}
