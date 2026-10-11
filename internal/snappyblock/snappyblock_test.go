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

func TestDecodeFramedRoundTrip(t *testing.T) {
	plain := bytes.Repeat([]byte(`{"content":"olá"}`), 100)
	var framed bytes.Buffer
	writer := snappy.NewBufferedWriter(&framed)
	_, _ = writer.Write(plain)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeFramed(framed.Bytes())
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatalf("expected round trip, got %d bytes (err %v)", len(decoded), err)
	}
}

func TestDecodeFramedRejectsABlock(t *testing.T) {
	if _, err := DecodeFramed(snappy.Encode(nil, []byte("abc"))); err == nil {
		t.Fatal("expected an error for a block without the stream header")
	}
}

func TestDecodeFramedIgnoresChunkChecksums(t *testing.T) {
	var framed bytes.Buffer
	writer := snappy.NewBufferedWriter(&framed)
	_, _ = writer.Write([]byte(`{"content":"olá"}`))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw := framed.Bytes()
	// The first chunk's checksum follows the 10-byte stream identifier
	// and the chunk's 4-byte header.
	copy(raw[14:18], []byte{1, 2, 3, 4})
	if decoded, err := DecodeFramed(raw); err != nil || string(decoded) != `{"content":"olá"}` {
		t.Fatalf("DecodeFramed = %q, %v; want the body despite the checksum", decoded, err)
	}
}
