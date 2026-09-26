package v8value

import (
	"fmt"
	"math/big"
	"strconv"
)

// V8 serialization tags (src/objects/value-serializer.cc).
const (
	tagVersion       = 0xFF
	tagPadding       = 0x00
	tagVerifyCount   = '?'
	tagUndefined     = '_'
	tagNull          = '0'
	tagTrue          = 'T'
	tagFalse         = 'F'
	tagInt32         = 'I'
	tagUint32        = 'U'
	tagDouble        = 'N'
	tagBigInt        = 'Z'
	tagUTF8String    = 'S'
	tagLatin1String  = '"'
	tagTwoByteString = 'c'
	tagReference     = '^'
	tagDate          = 'D'
	tagTheHole       = '-'
)

// maxNesting bounds recursion so hostile or corrupt input cannot exhaust
// the stack.
const maxNesting = 256

type tagDecoder func(d *decoder) (*Value, error)

// decoder holds the state of one Decode call. objects records every
// reference-able value in creation order, which is how '^' back-references
// address them.
type decoder struct {
	byteReader
	objects []*Value
	nesting int
}

// Decode parses a V8-serialized value, starting at its 0xFF version header.
//
//	root, err := v8value.Decode(payload)
func Decode(payload []byte) (*Value, error) {
	d := &decoder{byteReader: byteReader{data: payload}}
	if err := d.readHeader(); err != nil {
		return nil, err
	}
	return d.readValue()
}

func (d *decoder) readHeader() error {
	tag, err := d.readByte()
	if err != nil || tag != tagVersion {
		return fmt.Errorf("V8 header byte %#x, expected %#x (err %v)", tag, tagVersion, err)
	}
	_, err = d.readVarint()
	return err
}

func (d *decoder) readValue() (*Value, error) {
	if d.nesting >= maxNesting {
		return nil, fmt.Errorf("nesting deeper than %d at offset %d", maxNesting, d.pos)
	}
	d.nesting++
	defer func() { d.nesting-- }()
	tag, err := d.readSignificantTag()
	if err != nil {
		return nil, err
	}
	decode, known := tagDecoders[tag]
	if !known {
		return nil, fmt.Errorf("unsupported tag %q (%#x) at offset %d", tag, tag, d.pos-1)
	}
	return decode(d)
}

// readSignificantTag skips alignment padding and object-count checks, which
// carry no value.
func (d *decoder) readSignificantTag() (byte, error) {
	for {
		tag, err := d.readByte()
		if err != nil || (tag != tagPadding && tag != tagVerifyCount) {
			return tag, err
		}
		if tag == tagVerifyCount {
			if _, err := d.readVarint(); err != nil {
				return 0, err
			}
		}
	}
}

func (d *decoder) remember(value *Value) *Value {
	d.objects = append(d.objects, value)
	return value
}

var tagDecoders map[byte]tagDecoder

func init() {
	tagDecoders = map[byte]tagDecoder{
		tagUndefined: constant(KindUndefined, false), tagNull: constant(KindNull, false),
		tagTrue: constant(KindBool, true), tagFalse: constant(KindBool, false),
		tagInt32: decodeInt32, tagUint32: decodeUint32, tagDouble: decodeDouble, tagBigInt: decodeBigInt,
		tagUTF8String: decodeUTF8, tagLatin1String: decodeLatin1, tagTwoByteString: decodeTwoByte,
		tagReference: decodeReference, tagDate: decodeDate,
	}
	registerContainerDecoders(tagDecoders)
}

func constant(kind Kind, flag bool) tagDecoder {
	return func(*decoder) (*Value, error) { return &Value{Kind: kind, Bool: flag}, nil }
}

func decodeInt32(d *decoder) (*Value, error) {
	number, err := d.readZigZag()
	return &Value{Kind: KindNumber, Number: float64(int32(number))}, err
}

func decodeUint32(d *decoder) (*Value, error) {
	number, err := d.readVarint()
	return &Value{Kind: KindNumber, Number: float64(uint32(number))}, err
}

func decodeDouble(d *decoder) (*Value, error) {
	number, err := d.readDouble()
	return &Value{Kind: KindNumber, Number: number}, err
}

func decodeDate(d *decoder) (*Value, error) {
	millis, err := d.readDouble()
	return d.remember(&Value{Kind: KindDate, Number: millis}), err
}

// decodeBigInt reads the bitfield (bit 0 = sign, the rest = byte length ×2)
// and the little-endian magnitude, rendering it as decimal digits.
func decodeBigInt(d *decoder) (*Value, error) {
	bitfield, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	magnitude, err := d.readBytes(bitfield >> 1)
	if err != nil {
		return nil, err
	}
	number := new(big.Int).SetBytes(reversed(magnitude))
	if bitfield&1 == 1 {
		number.Neg(number)
	}
	return &Value{Kind: KindBigInt, Text: number.String()}, nil
}

func reversed(littleEndian []byte) []byte {
	bigEndian := make([]byte, len(littleEndian))
	for i, b := range littleEndian {
		bigEndian[len(littleEndian)-1-i] = b
	}
	return bigEndian
}

func decodeUTF8(d *decoder) (*Value, error) {
	raw, err := d.readLengthPrefixed()
	return &Value{Kind: KindString, Text: string(raw)}, err
}

func decodeLatin1(d *decoder) (*Value, error) {
	text, err := d.readLatin1()
	return &Value{Kind: KindString, Text: text}, err
}

func decodeTwoByte(d *decoder) (*Value, error) {
	text, err := d.readUTF16LE()
	return &Value{Kind: KindString, Text: text}, err
}

func decodeReference(d *decoder) (*Value, error) {
	id, err := d.readVarint()
	if err != nil {
		return nil, err
	}
	if id >= uint64(len(d.objects)) {
		return nil, fmt.Errorf("reference to object %d at offset %d, only %d objects seen", id, d.pos, len(d.objects))
	}
	return d.objects[id], nil
}

// propertyKey renders a property key: V8 writes integer-like keys as
// numbers and everything else as strings.
func propertyKey(key *Value) (string, error) {
	switch key.Kind {
	case KindString:
		return key.Text, nil
	case KindNumber:
		return strconv.FormatFloat(key.Number, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("property key of kind %s, expected string or number", key.Kind)
}
