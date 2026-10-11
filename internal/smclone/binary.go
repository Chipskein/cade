package smclone

import (
	"fmt"

	"github.com/chipskein/cade/internal/v8value"
)

// typedArrayElementSizes is indexed by js::Scalar::Type: Int8, Uint8,
// Int16, Uint16, Int32, Uint32, Float32, Float64, Uint8Clamped, BigInt64,
// BigUint64, Float16.
var typedArrayElementSizes = [...]uint64{1, 1, 2, 2, 4, 4, 4, 8, 1, 8, 8, 2}

// dataViewElementSize: a DataView's length is already in bytes.
const dataViewElementSize = 1

func registerBinaryDecoders(decoders map[uint32]tagDecoder) {
	decoders[tagArrayBufferObjectV2] = decodeArrayBufferV2
	decoders[tagArrayBufferObject] = decodeArrayBuffer
	decoders[tagResizableArrayBuffer] = decodeResizableArrayBuffer
	decoders[tagTypedArrayObjectV2] = decodeTypedArrayV2
	decoders[tagTypedArrayObject] = decodeTypedArray
	decoders[tagDataViewObjectV2] = decodeDataViewV2
	decoders[tagDataViewObject] = decodeDataView
}

// decodeArrayBufferV2 reads a buffer whose byte length is the pair data.
func decodeArrayBufferV2(d *decoder, length uint32) (*v8value.Value, error) {
	return d.readBuffer(uint64(length))
}

// decodeArrayBuffer reads a buffer whose byte length is the next word.
func decodeArrayBuffer(d *decoder, _ uint32) (*v8value.Value, error) {
	length, err := d.readWord()
	if err != nil {
		return nil, err
	}
	return d.readBuffer(length)
}

// decodeResizableArrayBuffer skips the maximum length that follows the
// current one; only the current bytes matter.
func decodeResizableArrayBuffer(d *decoder, _ uint32) (*v8value.Value, error) {
	length, err := d.readWord()
	if err != nil {
		return nil, err
	}
	if _, err := d.readWord(); err != nil {
		return nil, err
	}
	return d.readBuffer(length)
}

func (d *decoder) readBuffer(length uint64) (*v8value.Value, error) {
	raw, err := d.readPadded(length)
	if err != nil {
		return nil, err
	}
	return d.remember(&v8value.Value{Kind: v8value.KindBinary, Bytes: raw}), nil
}

// decodeTypedArrayV2 reads the old layout: element count in the pair data,
// element type in the next word.
func decodeTypedArrayV2(d *decoder, count uint32) (*v8value.Value, error) {
	arrayType, err := d.readWord()
	if err != nil {
		return nil, err
	}
	return d.readView(arrayType, uint64(count))
}

// decodeTypedArray reads the current layout: element type in the pair
// data, element count in the next word.
func decodeTypedArray(d *decoder, arrayType uint32) (*v8value.Value, error) {
	count, err := d.readWord()
	if err != nil {
		return nil, err
	}
	return d.readView(uint64(arrayType), count)
}

func decodeDataViewV2(d *decoder, length uint32) (*v8value.Value, error) {
	return d.readViewOf(dataViewElementSize, uint64(length))
}

func decodeDataView(d *decoder, _ uint32) (*v8value.Value, error) {
	length, err := d.readWord()
	if err != nil {
		return nil, err
	}
	return d.readViewOf(dataViewElementSize, length)
}

func (d *decoder) readView(arrayType, count uint64) (*v8value.Value, error) {
	if arrayType >= uint64(len(typedArrayElementSizes)) {
		return nil, fmt.Errorf("typed array type %d at offset %d, expected 0-%d", arrayType, d.pos, len(typedArrayElementSizes)-1)
	}
	return d.readViewOf(typedArrayElementSizes[arrayType], count)
}

// readViewOf reads the view's buffer (a value of its own, often a
// back-reference) and byte offset, keeping the slice the view covers. The
// view's back-reference slot comes before its buffer's.
func (d *decoder) readViewOf(elementSize, count uint64) (*v8value.Value, error) {
	slot := d.reserve()
	buffer, err := d.readValue()
	if err != nil {
		return nil, err
	}
	offset, err := d.readWord()
	if err != nil {
		return nil, err
	}
	if buffer.Kind != v8value.KindBinary || count > uint64(len(buffer.Bytes))/elementSize || offset > uint64(len(buffer.Bytes))-count*elementSize {
		return nil, fmt.Errorf("view of %d×%d bytes at offset %d does not fit a %s of %d bytes", count, elementSize, offset, buffer.Kind, len(buffer.Bytes))
	}
	view := &v8value.Value{Kind: v8value.KindBinary, Bytes: buffer.Bytes[offset : offset+count*elementSize]}
	d.objects[slot] = view
	return view, nil
}
