package httpbody

import (
	"bytes"
	"compress/zlib"
	"io"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

const plainBody = `[{"content":"olá, revisão às 15h"}]`

type compressFunc func(io.Writer) io.WriteCloser

func compress(t *testing.T, plain []byte, open compressFunc) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := open(&out)
	if _, err := writer.Write(plain); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close compressor: %v", err)
	}
	return out.Bytes()
}

var compressors = map[Encoding]compressFunc{
	EncodingGzip:    func(w io.Writer) io.WriteCloser { return gzip.NewWriter(w) },
	EncodingDeflate: func(w io.Writer) io.WriteCloser { return zlib.NewWriter(w) },
	EncodingBrotli:  func(w io.Writer) io.WriteCloser { return brotli.NewWriter(w) },
	EncodingZstd: func(w io.Writer) io.WriteCloser {
		encoder, _ := zstd.NewWriter(w)
		return encoder
	},
}

func TestDecodeUndoesEachEncoding(t *testing.T) {
	for encoding, open := range compressors {
		decoded, err := Decode(string(encoding), compress(t, []byte(plainBody), open))
		if err != nil || string(decoded) != plainBody {
			t.Errorf("Decode(%s) = %q, %v; want the plain body", encoding, decoded, err)
		}
	}
}

func TestDecodeReadsRawDeflate(t *testing.T) {
	raw := compress(t, []byte(plainBody), func(w io.Writer) io.WriteCloser {
		writer, _ := flate.NewWriter(w, flate.DefaultCompression)
		return writer
	})
	if decoded, err := Decode("deflate", raw); err != nil || string(decoded) != plainBody {
		t.Fatalf("Decode(raw deflate) = %q, %v", decoded, err)
	}
}

func TestDecodeUndoesLayersLastFirst(t *testing.T) {
	gzipped := compress(t, []byte(plainBody), compressors[EncodingGzip])
	layered := compress(t, gzipped, compressors[EncodingBrotli])
	if decoded, err := Decode("gzip, BR", layered); err != nil || string(decoded) != plainBody {
		t.Fatalf("Decode(gzip, br) = %q, %v", decoded, err)
	}
}

func TestDecodeKeepsIdentityAndEmpty(t *testing.T) {
	for _, header := range []string{"", "identity", " Identity "} {
		if decoded, err := Decode(header, []byte(plainBody)); err != nil || string(decoded) != plainBody {
			t.Errorf("Decode(%q) = %q, %v; want the body unchanged", header, decoded, err)
		}
	}
}

func TestDecodeNamesAnUnknownEncoding(t *testing.T) {
	_, err := Decode("compress", []byte(plainBody))
	if err == nil || !strings.Contains(err.Error(), `"compress"`) || !strings.Contains(err.Error(), "zstd") {
		t.Fatalf("Decode(compress) error = %v; want the value and the encodings expected", err)
	}
}

func TestDecodeReportsACorruptBody(t *testing.T) {
	if _, err := Decode("gzip", []byte("not gzip")); err == nil || !strings.Contains(err.Error(), "gzip") {
		t.Fatalf("Decode(corrupt gzip) error = %v; want one naming gzip", err)
	}
}

func TestDecodeRefusesABodyPastTheCap(t *testing.T) {
	bomb := compress(t, make([]byte, MaxDecodedBytes+1), compressors[EncodingGzip])
	if _, err := Decode("gzip", bomb); err == nil || !strings.Contains(err.Error(), "past") {
		t.Fatalf("Decode(bomb) error = %v; want the cap reported", err)
	}
}
