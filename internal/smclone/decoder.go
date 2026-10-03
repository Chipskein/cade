// Package smclone decodes SpiderMonkey's structured clone format (the
// value format Firefox uses for IndexedDB) into the same v8value.Value tree
// the Chromium decoder produces, so callers never care which engine wrote
// the data.
package smclone

import (
	"fmt"

	"github.com/chipskein/cade/internal/v8value"
)

// SpiderMonkey tags (js/src/vm/StructuredClone.cpp, StructuredDataType).
// A word whose high half is at most tagFloatMax is a raw double.
const (
	tagFloatMax               uint32 = 0xFFF00000
	tagHeader                 uint32 = 0xFFF10000
	tagNull                   uint32 = 0xFFFF0000
	tagUndefined              uint32 = 0xFFFF0001
	tagBoolean                uint32 = 0xFFFF0002
	tagInt32                  uint32 = 0xFFFF0003
	tagString                 uint32 = 0xFFFF0004
	tagDateObject             uint32 = 0xFFFF0005
	tagRegExpObject           uint32 = 0xFFFF0006
	tagArrayObject            uint32 = 0xFFFF0007
	tagObjectObject           uint32 = 0xFFFF0008
	tagArrayBufferObjectV2    uint32 = 0xFFFF0009
	tagBooleanObject          uint32 = 0xFFFF000A
	tagStringObject           uint32 = 0xFFFF000B
	tagNumberObject           uint32 = 0xFFFF000C
	tagBackReferenceObject    uint32 = 0xFFFF000D
	tagTypedArrayObjectV2     uint32 = 0xFFFF0010
	tagMapObject              uint32 = 0xFFFF0011
	tagSetObject              uint32 = 0xFFFF0012
	tagEndOfKeys              uint32 = 0xFFFF0013
	tagDataViewObjectV2       uint32 = 0xFFFF0015
	tagBigInt                 uint32 = 0xFFFF001D
	tagBigIntObject           uint32 = 0xFFFF001E
	tagArrayBufferObject      uint32 = 0xFFFF001F
	tagTypedArrayObject       uint32 = 0xFFFF0020
	tagDataViewObject         uint32 = 0xFFFF0021
	tagResizableArrayBuffer   uint32 = 0xFFFF0023
	tagDOMBlob                uint32 = 0xFFFF8001
	tagDOMFileWithoutLastMod  uint32 = 0xFFFF8002
	tagDOMFile                uint32 = 0xFFFF8005
	stringLatin1Flag          uint32 = 1 << 31
	stringLengthMask          uint32 = stringLatin1Flag - 1
	bigIntNegativeFlag               = stringLatin1Flag
	bigIntLengthMask                 = stringLengthMask
	maxNesting                       = 256
	maxSparseItems                   = 1 << 16
	propertyKeyFloatPrecision        = -1
)

// tagDecoder decodes the value whose pair (tag, data) was just read.
type tagDecoder func(d *decoder, data uint32) (*v8value.Value, error)

var tagDecoders map[uint32]tagDecoder

func init() {
	tagDecoders = map[uint32]tagDecoder{}
	registerPrimitiveDecoders(tagDecoders)
	registerContainerDecoders(tagDecoders)
	registerBinaryDecoders(tagDecoders)
	registerDOMDecoders(tagDecoders)
}

// decoder holds the state of one Decode call. objects records every
// object in creation order, which is how back-references address them.
type decoder struct {
	pairReader
	objects []*v8value.Value
	nesting int
}

// Decode parses one uncompressed structured clone buffer.
//
//	root, err := smclone.Decode(payload)
func Decode(payload []byte) (*v8value.Value, error) {
	d := &decoder{pairReader: pairReader{data: payload}}
	if err := d.skipHeader(); err != nil {
		return nil, err
	}
	return d.readValue()
}

// skipHeader consumes the optional SCTAG_HEADER pair, whose data is the
// clone scope and carries no value.
func (d *decoder) skipHeader() error {
	tag, err := d.peekTag()
	if err != nil || tag != tagHeader {
		return err
	}
	_, err = d.readWord()
	return err
}

func (d *decoder) readValue() (*v8value.Value, error) {
	if d.nesting >= maxNesting {
		return nil, fmt.Errorf("nesting deeper than %d at offset %d", maxNesting, d.pos)
	}
	d.nesting++
	defer func() { d.nesting-- }()
	start := d.pos
	word, err := d.readWord()
	if err != nil {
		return nil, err
	}
	return d.decodeWord(word, start)
}

func (d *decoder) decodeWord(word uint64, start int) (*v8value.Value, error) {
	tag, data := uint32(word>>32), uint32(word)
	if tag <= tagFloatMax {
		d.pos = start
		number, err := d.readDouble()
		return &v8value.Value{Kind: v8value.KindNumber, Number: number}, err
	}
	decode, known := tagDecoders[tag]
	if !known {
		return nil, fmt.Errorf("unsupported tag %#x at offset %d, expected a SpiderMonkey value tag", tag, start)
	}
	return decode(d, data)
}

func (d *decoder) remember(value *v8value.Value) *v8value.Value {
	d.objects = append(d.objects, value)
	return value
}

// reserve holds a back-reference slot for an object whose contents are
// read after its children (typed arrays read their buffer first).
func (d *decoder) reserve() int {
	d.objects = append(d.objects, nil)
	return len(d.objects) - 1
}
