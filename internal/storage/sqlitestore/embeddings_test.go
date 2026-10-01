package sqlitestore

import (
	"slices"
	"testing"
)

func TestEncodeInt8VectorUsesTheWholeRange(t *testing.T) {
	got := encodeInt8Vector([]float32{0.1, -0.05, 0})
	want := []byte{127, 0xc0 /* int8(-64) */, 0}
	if !slices.Equal(got, want) {
		t.Errorf("encodeInt8Vector([0.1 -0.05 0]) = %v, want %v", got, want)
	}
	if zero := encodeInt8Vector([]float32{0, 0}); !slices.Equal(zero, []byte{0, 0}) {
		t.Errorf("encodeInt8Vector of a zero vector = %v, want zeros", zero)
	}
}

func TestDecodeInt8VectorIsAUnitVector(t *testing.T) {
	got := decodeInt8Vector([]byte{3, 0xfc /* int8(-4) */})
	if want := []float32{0.6, -0.8}; !slices.Equal(got, want) {
		t.Errorf("decodeInt8Vector([3 -4]) = %v, want %v", got, want)
	}
	if zero := decodeInt8Vector([]byte{0, 0}); !slices.Equal(zero, []float32{0, 0}) {
		t.Errorf("decodeInt8Vector of a zero blob = %v, want zeros", zero)
	}
}

// A vector read back and stored again, as ingestion does when it reuses
// the vector of identical text, must not drift.
func TestInt8VectorSurvivesARoundTrip(t *testing.T) {
	stored := encodeInt8Vector([]float32{0.02, -0.17, 0.09, 0.005})
	if again := encodeInt8Vector(decodeInt8Vector(stored)); !slices.Equal(again, stored) {
		t.Errorf("encode(decode(%v)) = %v, want the same blob", stored, again)
	}
}
