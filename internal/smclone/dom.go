package smclone

import (
	"encoding/binary"
	"fmt"

	"github.com/chipskein/cade/internal/v8value"
)

// lengthFieldSize is the uint32 length IndexedDB writes before a Blob's
// type and a File's name (dom/indexedDB/IDBObjectStore.cpp), padded to a
// word like every byte run.
const lengthFieldSize = 4

// Keys of the object a Blob or File decodes into; the bytes live in the
// idb .files directory, so only the description is kept.
const (
	blobSizeKey         = "size"
	blobTypeKey         = "type"
	fileNameKey         = "name"
	fileLastModifiedKey = "lastModified"
)

// blobLayout says which optional File fields follow a Blob's size and type.
type blobLayout struct {
	lastModified bool
	name         bool
}

func registerDOMDecoders(decoders map[uint32]tagDecoder) {
	decoders[tagDOMBlob] = blobDecoder(blobLayout{})
	decoders[tagDOMFileWithoutLastMod] = blobDecoder(blobLayout{name: true})
	decoders[tagDOMFile] = blobDecoder(blobLayout{lastModified: true, name: true})
}

// blobDecoder reads a Blob or File description; the pair data indexes the
// record's file_ids and is not kept.
func blobDecoder(layout blobLayout) tagDecoder {
	return func(d *decoder, _ uint32) (*v8value.Value, error) {
		blob := d.remember(&v8value.Value{Kind: v8value.KindObject})
		size, err := d.readWord()
		if err != nil {
			return nil, err
		}
		blob.Properties = append(blob.Properties, numberProperty(blobSizeKey, float64(size)))
		if err := d.appendText(blob, blobTypeKey); err != nil {
			return nil, err
		}
		return blob, d.readFileFields(blob, layout)
	}
}

func (d *decoder) readFileFields(blob *v8value.Value, layout blobLayout) error {
	if layout.lastModified {
		millis, err := d.readWord()
		if err != nil {
			return err
		}
		blob.Properties = append(blob.Properties, numberProperty(fileLastModifiedKey, float64(int64(millis))))
	}
	if !layout.name {
		return nil
	}
	return d.appendText(blob, fileNameKey)
}

// appendText reads a uint32-length-prefixed UTF-8 run into a property.
func (d *decoder) appendText(object *v8value.Value, key string) error {
	field, err := d.readPadded(lengthFieldSize)
	if err != nil {
		return fmt.Errorf("read %s length: %w", key, err)
	}
	text, err := d.readPadded(uint64(binary.LittleEndian.Uint32(field)))
	if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	object.Properties = append(object.Properties, v8value.Property{Key: key, Value: &v8value.Value{Kind: v8value.KindString, Text: string(text)}})
	return nil
}

func numberProperty(key string, number float64) v8value.Property {
	return v8value.Property{Key: key, Value: &v8value.Value{Kind: v8value.KindNumber, Number: number}}
}
