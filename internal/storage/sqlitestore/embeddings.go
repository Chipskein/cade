package sqlitestore

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Vectors are stored as int8, each scaled by its own largest component so
// ±127 covers the range it uses (#66). Cosine ignores the scale, so the
// search compares the same directions as in float32; sqlite-vec's 'unit'
// scale left most of the 256 levels unused, since normalized 768-dim
// vectors rarely pass ±0.2, and lost 4.5% of the top-10 (#40).

// int8VectorValue turns a bound int8 blob into a vec0 int8 value; without
// it vec0 reads the blob as float32.
const int8VectorValue = "vec_int8(?)"

// encodeInt8Vector scales vector so its largest component is ±127; a zero
// vector stays zero.
//
//	encodeInt8Vector([]float32{0.1, -0.05}) // []byte{127, 0xc0} (int8 -64)
func encodeInt8Vector(vector []float32) []byte {
	largest := largestMagnitude(vector)
	blob := make([]byte, len(vector))
	if largest == 0 {
		return blob
	}
	for i, value := range vector {
		blob[i] = byte(int8(math.Round(float64(value) * math.MaxInt8 / largest)))
	}
	return blob
}

func largestMagnitude(vector []float32) float64 {
	var largest float64
	for _, value := range vector {
		largest = math.Max(largest, math.Abs(float64(value)))
	}
	return largest
}

// decodeInt8Vector returns the stored direction as a unit vector, since
// callers score it by dot product (rag.cosineDistance); encoding it again
// gives the same blob.
//
//	decodeInt8Vector([]byte{3, 4}) // []float32{0.6, 0.8}
func decodeInt8Vector(blob []byte) []float32 {
	var squares float64
	for _, component := range blob {
		squares += float64(int8(component)) * float64(int8(component))
	}
	vector := make([]float32, len(blob))
	if squares == 0 {
		return vector
	}
	norm := math.Sqrt(squares)
	for i, component := range blob {
		vector[i] = float32(float64(int8(component)) / norm)
	}
	return vector
}

// decodeFloat32s reads sqlite-vec's float32 blob: little-endian IEEE 754,
// the format of vectors stored before schema version 11.
func decodeFloat32s(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("embedding blob of %d bytes, expected a multiple of 4", len(blob))
	}
	vector := make([]float32, len(blob)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[4*i:]))
	}
	return vector, nil
}
