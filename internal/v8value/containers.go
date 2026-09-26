package v8value

import (
	"fmt"
	"strconv"
)

// Container and wrapper tags. Begin tags open a container; the matching end
// tag closes it and is followed by counts that are only redundant checks.
const (
	tagBeginObject      = 'o'
	tagEndObject        = '{'
	tagBeginSparseArray = 'a'
	tagEndSparseArray   = '@'
	tagBeginDenseArray  = 'A'
	tagEndDenseArray    = '$'
	tagBeginMap         = ';'
	tagEndMap           = ':'
	tagBeginSet         = '\''
	tagEndSet           = ','
	tagTrueObject       = 'y'
	tagFalseObject      = 'x'
	tagNumberObject     = 'n'
	tagBigIntObject     = 'z'
	tagStringObject     = 's'
	tagRegExp           = 'R'
	tagArrayBuffer      = 'B'
	tagArrayBufferView  = 'V'
)

func registerContainerDecoders(decoders map[byte]tagDecoder) {
	decoders[tagBeginObject] = decodeObject
	decoders[tagBeginSparseArray] = decodeSparseArray
	decoders[tagBeginDenseArray] = decodeDenseArray
	decoders[tagBeginMap] = decodeMap
	decoders[tagBeginSet] = decodeSet
	decoders[tagTrueObject] = boxed(constant(KindBool, true))
	decoders[tagFalseObject] = boxed(constant(KindBool, false))
	decoders[tagNumberObject] = boxed(decodeDouble)
	decoders[tagBigIntObject] = boxed(decodeBigInt)
	decoders[tagStringObject] = boxed(func(d *decoder) (*Value, error) { return d.readValue() })
	decoders[tagRegExp] = decodeRegExp
	decoders[tagArrayBuffer] = decodeArrayBuffer
}

// boxed decodes wrapper objects (new String("x")...) as their primitive,
// but still registers them for back-references.
func boxed(decodePrimitive tagDecoder) tagDecoder {
	return func(d *decoder) (*Value, error) {
		value, err := decodePrimitive(d)
		if err != nil {
			return nil, err
		}
		return d.remember(value), nil
	}
}

func decodeObject(d *decoder) (*Value, error) {
	object := d.remember(&Value{Kind: KindObject})
	properties, err := d.readProperties(tagEndObject)
	if err != nil {
		return nil, err
	}
	object.Properties = properties
	_, err = d.readVarint()
	return object, err
}

// readProperties reads key/value pairs until endTag, consuming it.
func (d *decoder) readProperties(endTag byte) ([]Property, error) {
	var properties []Property
	for {
		ended, err := d.consumeIfNext(endTag)
		if err != nil || ended {
			return properties, err
		}
		property, err := d.readProperty()
		if err != nil {
			return nil, err
		}
		properties = append(properties, property)
	}
}

func (d *decoder) readProperty() (Property, error) {
	key, err := d.readValue()
	if err != nil {
		return Property{}, err
	}
	name, err := propertyKey(key)
	if err != nil {
		return Property{}, fmt.Errorf("at offset %d: %w", d.pos, err)
	}
	value, err := d.readValue()
	return Property{Key: name, Value: value}, err
}

// consumeIfNext skips padding and reports whether the next tag is tag,
// consuming it if so.
func (d *decoder) consumeIfNext(tag byte) (bool, error) {
	for {
		next, ok := d.peekByte()
		if !ok {
			return false, fmt.Errorf("data ended at offset %d while looking for tag %q", d.pos, tag)
		}
		if next != tagPadding {
			d.pos += boolToOffset(next == tag)
			return next == tag, nil
		}
		d.pos++
	}
}

func boolToOffset(consume bool) int {
	if consume {
		return 1
	}
	return 0
}

// decodeDenseArray reads length elements (holes stay nil), then any extra
// named properties.
func decodeDenseArray(d *decoder) (*Value, error) {
	array := d.remember(&Value{Kind: KindArray})
	length, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	for index := uint64(0); index < length; index++ {
		element, err := d.readArrayElement()
		if err != nil {
			return nil, err
		}
		array.Items = append(array.Items, element)
	}
	return array, d.finishArray(array, tagEndDenseArray)
}

func (d *decoder) readArrayElement() (*Value, error) {
	hole, err := d.consumeIfNext(tagTheHole)
	if err != nil || hole {
		return nil, err
	}
	return d.readValue()
}

// decodeSparseArray stores index properties as items and the rest as
// properties, so both array shapes look the same to callers.
func decodeSparseArray(d *decoder) (*Value, error) {
	array := d.remember(&Value{Kind: KindArray})
	length, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if err := d.finishArray(array, tagEndSparseArray); err != nil {
		return nil, err
	}
	promoteIndexProperties(array, length)
	return array, nil
}

// maxSparseItems caps the Items slice of sparse arrays: a JS array can be
// declared with length 2^32-1 while holding a single element.
const maxSparseItems = 1 << 16

func promoteIndexProperties(array *Value, length uint64) {
	array.Items = make([]*Value, min(length, maxSparseItems))
	var named []Property
	for _, property := range array.Properties {
		index, err := strconv.ParseUint(property.Key, 10, 64)
		if err != nil || index >= uint64(len(array.Items)) {
			named = append(named, property)
			continue
		}
		array.Items[index] = property.Value
	}
	array.Properties = named
}

func (d *decoder) finishArray(array *Value, endTag byte) error {
	properties, err := d.readProperties(endTag)
	if err != nil {
		return err
	}
	array.Properties = properties
	if _, err := d.readVarint(); err != nil {
		return err
	}
	_, err = d.readVarint()
	return err
}

func decodeMap(d *decoder) (*Value, error) {
	mapping := d.remember(&Value{Kind: KindMap})
	for {
		ended, err := d.consumeIfNext(tagEndMap)
		if err != nil {
			return nil, err
		}
		if ended {
			_, err = d.readVarint()
			return mapping, err
		}
		if err := d.readMapEntry(mapping); err != nil {
			return nil, err
		}
	}
}

func (d *decoder) readMapEntry(mapping *Value) error {
	key, err := d.readValue()
	if err != nil {
		return err
	}
	value, err := d.readValue()
	mapping.Entries = append(mapping.Entries, MapEntry{Key: key, Value: value})
	return err
}

func decodeSet(d *decoder) (*Value, error) {
	set := d.remember(&Value{Kind: KindSet})
	for {
		ended, err := d.consumeIfNext(tagEndSet)
		if err != nil {
			return nil, err
		}
		if ended {
			_, err = d.readVarint()
			return set, err
		}
		member, err := d.readValue()
		if err != nil {
			return nil, err
		}
		set.Items = append(set.Items, member)
	}
}

func decodeRegExp(d *decoder) (*Value, error) {
	regexp := d.remember(&Value{Kind: KindRegExp})
	source, err := d.readValue()
	if err != nil {
		return nil, err
	}
	regexp.Text = source.Text
	_, err = d.readVarint()
	return regexp, err
}

// decodeArrayBuffer reads the buffer and, when a view follows, the viewed
// slice (typed arrays are serialized as buffer + view).
func decodeArrayBuffer(d *decoder) (*Value, error) {
	raw, err := d.readLengthPrefixed()
	if err != nil {
		return nil, err
	}
	buffer := d.remember(&Value{Kind: KindBinary, Bytes: raw})
	isView, err := d.consumeIfNext(tagArrayBufferView)
	if err != nil || !isView {
		return buffer, err
	}
	return d.readArrayBufferView(raw)
}

// readArrayBufferView reads subtag (element type), byte offset, byte
// length and flags (present since format v14).
func (d *decoder) readArrayBufferView(buffer []byte) (*Value, error) {
	subtag, err := d.readByte()
	if err != nil {
		return nil, err
	}
	offset, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	length, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if offset+length > uint64(len(buffer)) {
		return nil, fmt.Errorf("view %q of %d bytes at %d exceeds %d-byte buffer", subtag, length, offset, len(buffer))
	}
	if _, err := d.readVarint(); err != nil {
		return nil, err
	}
	return d.remember(&Value{Kind: KindBinary, Bytes: buffer[offset : offset+length]}), nil
}
