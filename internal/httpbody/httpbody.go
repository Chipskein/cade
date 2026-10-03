// Package httpbody undoes the Content-Encoding of an HTTP response body as
// browsers keep it in their HTTP cache: compressed, as it came over the
// network. It is the project's only dependency point on gzip, deflate,
// brotli and zstd implementations.
package httpbody

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

// Encoding is one Content-Encoding token.
type Encoding string

const (
	EncodingIdentity Encoding = "identity"
	EncodingGzip     Encoding = "gzip"
	EncodingDeflate  Encoding = "deflate"
	EncodingBrotli   Encoding = "br"
	EncodingZstd     Encoding = "zstd"
)

// MaxDecodedBytes caps a decoded body: a cached response is a page of
// messages, and a body that inflates past this is not one (or is a bomb).
const MaxDecodedBytes = 64 << 20

type openDecoder func(compressed io.Reader) (io.Reader, error)

var decoders = map[Encoding]openDecoder{
	EncodingGzip:    func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) },
	EncodingDeflate: openDeflate,
	EncodingBrotli:  func(r io.Reader) (io.Reader, error) { return brotli.NewReader(r), nil },
	EncodingZstd:    openZstd,
}

// Decode undoes contentEncoding, the header value as sent ("gzip", "br",
// "gzip, br" for two layers, "" for none).
//
//	plain, err := httpbody.Decode("br", cachedBody)
func Decode(contentEncoding string, body []byte) ([]byte, error) {
	encodings := parseEncodings(contentEncoding)
	// The header lists encodings in the order they were applied, so they
	// are undone last first.
	for _, encoding := range slices.Backward(encodings) {
		decoded, err := decodeOne(encoding, body)
		if err != nil {
			return nil, err
		}
		body = decoded
	}
	return body, nil
}

func parseEncodings(contentEncoding string) []Encoding {
	var encodings []Encoding
	for token := range strings.SplitSeq(contentEncoding, ",") {
		encoding := Encoding(strings.ToLower(strings.TrimSpace(token)))
		if encoding != "" && encoding != EncodingIdentity {
			encodings = append(encodings, encoding)
		}
	}
	return encodings
}

func decodeOne(encoding Encoding, body []byte) ([]byte, error) {
	open, known := decoders[encoding]
	if !known {
		return nil, fmt.Errorf("content encoding %q, expected one of %s, %s, %s, %s or %s", encoding, EncodingIdentity, EncodingGzip, EncodingDeflate, EncodingBrotli, EncodingZstd)
	}
	reader, err := open(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("open %s body of %d bytes: %w", encoding, len(body), err)
	}
	return readCapped(encoding, reader)
}

func readCapped(encoding Encoding, reader io.Reader) ([]byte, error) {
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, MaxDecodedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("decode %s body: %w", encoding, err)
	}
	if len(decoded) > MaxDecodedBytes {
		return nil, fmt.Errorf("%s body decodes past %d bytes, expected at most that", encoding, MaxDecodedBytes)
	}
	return decoded, nil
}

// openDeflate reads "deflate" both ways servers send it: zlib-wrapped, as
// the RFC says, or raw, as some servers do anyway.
func openDeflate(compressed io.Reader) (io.Reader, error) {
	raw, err := io.ReadAll(compressed)
	if err != nil {
		return nil, err
	}
	if zlibReader, err := zlib.NewReader(bytes.NewReader(raw)); err == nil {
		return zlibReader, nil
	}
	return flate.NewReader(bytes.NewReader(raw)), nil
}

// openZstd decodes on the calling goroutine: the default decoder starts
// workers that would outlive the body unless closed.
func openZstd(compressed io.Reader) (io.Reader, error) {
	decoder, err := zstd.NewReader(compressed, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	return decoder.IOReadCloser(), nil
}
