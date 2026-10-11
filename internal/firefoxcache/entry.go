// Package firefoxcache reads the request caches of Firefox and Floorp: the
// HTTP cache (cache2, in the profile's cache directory) and the Cache API
// (cache/, under an origin's storage/default directory).
package firefoxcache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/webstore"
)

// A cache2 entry file is the body as it came over the network, then the
// metadata, then the metadata's offset (netwerk/cache2/CacheFileMetadata.h).
// Every number is a big-endian uint32.
const (
	offsetSize = 4
	// The metadata opens with its own hash and one hash per body chunk.
	metadataHashSize = 4
	chunkHashSize    = 2
	chunkSize        = 256 * 1024
	// The header is version, fetch count, last fetched, last modified,
	// frecency, expiration time, key size and flags.
	headerSize        = 8 * 4
	keySizeOffset     = 6 * 4
	minVersion        = 2 // the first with flags in the header
	elementSeparator  = "\x00"
	responseHead      = "response-head"
	responseHeadLines = "\r\n"
	// A key is "<tags>:<url>"; its tags (anonymous, partition key, …) are
	// URL-escaped, so the first ':' ends them.
	keyURLSeparator = ":"
)

// entryMetadata is what a reader needs from an entry's metadata.
type entryMetadata struct {
	url      string
	elements map[string]string
}

// readEntry reads the entry in file when its URL is in scope, and reads
// its body only then: a cache2 file of any other site is never read past
// its metadata.
func readEntry(file io.ReaderAt, size int64, scope requestcache.Scope) (webstore.Record, bool, error) {
	bodySize, metadata, err := readMetadata(file, size)
	if err != nil {
		return webstore.Record{}, false, err
	}
	name, inScope := scope.Match(metadata.url)
	if !inScope {
		return webstore.Record{}, false, nil
	}
	body := make([]byte, bodySize)
	if _, err := file.ReadAt(body, 0); err != nil {
		return webstore.Record{}, false, fmt.Errorf("read %d-byte body of %s: %w", bodySize, metadata.url, err)
	}
	lines := strings.Split(metadata.elements[responseHead], responseHeadLines)
	response := requestcache.Response{URL: metadata.url, ContentEncoding: requestcache.HeaderValue(lines, requestcache.HeaderContentEncoding), Body: body}
	return response.Record(webstore.KindHTTPCache, name), true, nil
}

// readMetadata reads the metadata at the offset the entry ends with,
// which is also the body's size.
func readMetadata(file io.ReaderAt, size int64) (int64, entryMetadata, error) {
	if size < offsetSize {
		return 0, entryMetadata{}, fmt.Errorf("cache2 entry of %d bytes, expected at least %d", size, offsetSize)
	}
	tail, err := readAt(file, size-offsetSize, offsetSize)
	if err != nil {
		return 0, entryMetadata{}, err
	}
	bodySize := int64(binary.BigEndian.Uint32(tail))
	if bodySize > size-offsetSize {
		return 0, entryMetadata{}, fmt.Errorf("metadata offset %d in a %d-byte entry, expected one inside it", bodySize, size)
	}
	raw, err := readAt(file, bodySize, size-offsetSize-bodySize)
	if err != nil {
		return 0, entryMetadata{}, err
	}
	metadata, err := parseMetadata(raw, bodySize)
	return bodySize, metadata, err
}

func readAt(file io.ReaderAt, offset, length int64) ([]byte, error) {
	raw := make([]byte, length)
	if _, err := file.ReadAt(raw, offset); err != nil {
		return nil, fmt.Errorf("read %d bytes at %d of a cache2 entry: %w", length, offset, err)
	}
	return raw, nil
}

func parseMetadata(raw []byte, bodySize int64) (entryMetadata, error) {
	chunks := (bodySize + chunkSize - 1) / chunkSize
	header := metadataHashSize + int(chunks)*chunkHashSize
	if len(raw) < header+headerSize {
		return entryMetadata{}, fmt.Errorf("metadata of %d bytes, expected at least %d", len(raw), header+headerSize)
	}
	if version := binary.BigEndian.Uint32(raw[header:]); version < minVersion {
		return entryMetadata{}, fmt.Errorf("cache2 metadata version %d, expected %d or later", version, minVersion)
	}
	keyStart := header + headerSize
	keyEnd := keyStart + int(binary.BigEndian.Uint32(raw[header+keySizeOffset:]))
	// The key is followed by a NUL before the elements.
	if keyEnd >= len(raw) {
		return entryMetadata{}, fmt.Errorf("key ends at %d in %d bytes of metadata, expected it inside", keyEnd, len(raw))
	}
	_, url, _ := strings.Cut(string(raw[keyStart:keyEnd]), keyURLSeparator)
	return entryMetadata{url: url, elements: parseElements(raw[keyEnd+1:])}, nil
}

// parseElements reads the "name\0value\0" pairs after the key.
func parseElements(raw []byte) map[string]string {
	fields := bytes.Split(raw, []byte(elementSeparator))
	elements := make(map[string]string, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		elements[string(fields[i])] = string(fields[i+1])
	}
	return elements
}
