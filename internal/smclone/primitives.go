package smclone

import (
	"fmt"
	"math/big"

	"github.com/chipskein/cade/internal/v8value"
)

func registerPrimitiveDecoders(decoders map[uint32]tagDecoder) {
	decoders[tagNull] = constant(v8value.KindNull)
	decoders[tagUndefined] = constant(v8value.KindUndefined)
	decoders[tagBoolean] = decodeBoolean
	decoders[tagInt32] = decodeInt32
	decoders[tagString] = decodeString
	decoders[tagBigInt] = decodeBigInt
	decoders[tagBooleanObject] = boxed(decodeBoolean)
	decoders[tagStringObject] = boxed(decodeString)
	decoders[tagBigIntObject] = boxed(decodeBigInt)
	decoders[tagNumberObject] = boxed(decodeNumber)
	decoders[tagDateObject] = decodeDate
	decoders[tagRegExpObject] = decodeRegExp
}

func constant(kind v8value.Kind) tagDecoder {
	return func(*decoder, uint32) (*v8value.Value, error) { return &v8value.Value{Kind: kind}, nil }
}

// boxed decodes a wrapper object (new String("x")) as its primitive; it
// still takes a back-reference slot, as the wrapper is an object.
func boxed(decodePrimitive tagDecoder) tagDecoder {
	return func(d *decoder, data uint32) (*v8value.Value, error) {
		value, err := decodePrimitive(d, data)
		if err != nil {
			return nil, err
		}
		return d.remember(value), nil
	}
}

func decodeBoolean(_ *decoder, data uint32) (*v8value.Value, error) {
	return &v8value.Value{Kind: v8value.KindBool, Bool: data != 0}, nil
}

func decodeInt32(_ *decoder, data uint32) (*v8value.Value, error) {
	return &v8value.Value{Kind: v8value.KindNumber, Number: float64(int32(data))}, nil
}

// decodeNumber reads the double that follows a Number object's pair.
func decodeNumber(d *decoder, _ uint32) (*v8value.Value, error) {
	number, err := d.readDouble()
	return &v8value.Value{Kind: v8value.KindNumber, Number: number}, err
}

// decodeString reads the characters described by data: the length in the
// low 31 bits, Latin-1 when the top bit is set and UTF-16 otherwise.
func decodeString(d *decoder, data uint32) (*v8value.Value, error) {
	length := uint64(data & stringLengthMask)
	read := d.readTwoByte
	if data&stringLatin1Flag != 0 {
		read = d.readLatin1
	}
	text, err := read(length)
	return &v8value.Value{Kind: v8value.KindString, Text: text}, err
}

func decodeDate(d *decoder, _ uint32) (*v8value.Value, error) {
	millis, err := d.readDouble()
	return d.remember(&v8value.Value{Kind: v8value.KindDate, Number: millis}), err
}

// decodeBigInt reads data's length in 64-bit digits (least significant
// first) and its sign bit, rendering the number as decimal digits.
func decodeBigInt(d *decoder, data uint32) (*v8value.Value, error) {
	length := uint64(data & bigIntLengthMask)
	if length > uint64(len(d.data)-d.pos)/wordSize {
		return nil, fmt.Errorf("bigint of %d digits at offset %d exceeds the %d bytes left", length, d.pos, len(d.data)-d.pos)
	}
	number := new(big.Int)
	for i := uint64(0); i < length; i++ {
		digit, err := d.readWord()
		if err != nil {
			return nil, err
		}
		number.Or(number, new(big.Int).Lsh(new(big.Int).SetUint64(digit), uint(64*i)))
	}
	if data&bigIntNegativeFlag != 0 {
		number.Neg(number)
	}
	return &v8value.Value{Kind: v8value.KindBigInt, Text: number.String()}, nil
}

// decodeRegExp reads the source string pair that follows the flags pair.
func decodeRegExp(d *decoder, _ uint32) (*v8value.Value, error) {
	regexp := d.remember(&v8value.Value{Kind: v8value.KindRegExp})
	word, err := d.readWord()
	if err != nil {
		return nil, err
	}
	if tag := uint32(word >> 32); tag != tagString {
		return nil, fmt.Errorf("regexp source tagged %#x at offset %d, expected string tag %#x", tag, d.pos-wordSize, tagString)
	}
	source, err := decodeString(d, uint32(word))
	if err != nil {
		return nil, err
	}
	regexp.Text = source.Text
	return regexp, nil
}
