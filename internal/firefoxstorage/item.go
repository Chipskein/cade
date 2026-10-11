package firefoxstorage

import (
	"fmt"

	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/snappyblock"
	"github.com/chipskein/cade/internal/webstore"
)

// How a value is kept in data.sqlite (dom/localstorage/LSValue.h): its
// UTF-16 text converted to UTF-8, or the UTF-16 bytes as they are when the
// text does not convert; then Snappy-compressed or not.
const (
	conversionNone  = 0
	conversionUTF8  = 1
	compressionNone = 0
	compressionSnap = 1
)

// storedItem is one row of the data table.
type storedItem struct {
	key         string
	conversion  int
	compression int
	value       []byte
}

// record decodes the item; one that does not decode is still a record,
// with DecodeErr, so schema checks count it.
func (item storedItem) record() webstore.Record {
	record := webstore.Record{Kind: webstore.KindLocalStorage, Container: item.key, Key: item.key}
	text, err := item.text()
	if err != nil {
		record.DecodeErr = fmt.Errorf("localStorage item %q: %w", item.key, err)
		return record
	}
	record.Value = sitestorage.TextValue(text)
	return record
}

func (item storedItem) text() (string, error) {
	raw, err := uncompressed(item.compression, item.value)
	if err != nil {
		return "", err
	}
	switch item.conversion {
	case conversionUTF8:
		return string(raw), nil
	case conversionNone:
		return sitestorage.UTF16LE(raw)
	}
	return "", fmt.Errorf("conversion type %d, expected %d (none) or %d (UTF-8)", item.conversion, conversionNone, conversionUTF8)
}

func uncompressed(compression int, value []byte) ([]byte, error) {
	switch compression {
	case compressionNone:
		return value, nil
	case compressionSnap:
		return snappyblock.Decode(value)
	}
	return nil, fmt.Errorf("compression type %d, expected %d (none) or %d (Snappy)", compression, compressionNone, compressionSnap)
}
