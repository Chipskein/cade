package snappyblock

import (
	"bytes"
	"testing"

	"github.com/klauspost/compress/snappy"
)

func TestDecodeRoundTrip(t *testing.T) {
	plain := bytes.Repeat([]byte("abc"), 100)
	decoded, err := Decode(snappy.Encode(nil, plain))
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatalf("expected round trip, got %d bytes (err %v)", len(decoded), err)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := Decode([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0x01}); err == nil {
		t.Fatal("expected an error for invalid snappy data")
	}
}
