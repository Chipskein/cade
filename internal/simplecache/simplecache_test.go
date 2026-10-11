package simplecache_test

import (
	"bytes"
	"testing"

	"github.com/chipskein/cade/internal/simplecache"
	"github.com/chipskein/cade/internal/testfakes"
)

const messagesKey = "1/0/_dk_https://discord.com https://discord.com https://discord.com/api/v9/channels/42/messages"

func sampleEntry() testfakes.SimpleCacheEntry {
	return testfakes.SimpleCacheEntry{Key: messagesKey, Stream0: []byte("HTTP/1.1 200\x00\x00"), Body: []byte(`[{"id":"7"}]`)}
}

func TestOpenEntryReadsTheKey(t *testing.T) {
	raw := sampleEntry().Bytes()
	entry, err := simplecache.OpenEntry(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || entry.Key != messagesKey {
		t.Fatalf("OpenEntry = %q, %v; want the key", entry.Key, err)
	}
}

func TestStreamsSplitsHeadersAndBody(t *testing.T) {
	sample := sampleEntry()
	raw := sample.Bytes()
	entry, err := simplecache.OpenEntry(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	headers, body, err := entry.Streams()
	if err != nil || !bytes.Equal(headers, sample.Stream0) || !bytes.Equal(body, sample.Body) {
		t.Fatalf("Streams = %q, %q, %v; want stream 0 and the body", headers, body, err)
	}
}

func TestOpenEntryRefusesAnotherFile(t *testing.T) {
	raw := bytes.Repeat([]byte{7}, simplecache.HeaderSize)
	if _, err := simplecache.OpenEntry(bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("OpenEntry accepted a file without the magic")
	}
}

func TestStreamsRefusesATruncatedEntry(t *testing.T) {
	raw := sampleEntry().Bytes()
	cut := raw[:len(raw)-simplecache.EOFSize/2]
	entry, err := simplecache.OpenEntry(bytes.NewReader(cut), int64(len(cut)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := entry.Streams(); err == nil {
		t.Fatal("Streams read a truncated entry")
	}
}

func TestOpenEntryRefusesAKeyPastTheEnd(t *testing.T) {
	raw := sampleEntry().Bytes()[:simplecache.HeaderSize+4]
	if _, err := simplecache.OpenEntry(bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("OpenEntry read a key past the end")
	}
}

func TestEntryFilePatternKeepsStreamZeroFiles(t *testing.T) {
	if !simplecache.EntryFilePattern.MatchString("0d1c857aa3989262_0") || simplecache.EntryFilePattern.MatchString("0d1c857aa3989262_s") || simplecache.EntryFilePattern.MatchString("index") {
		t.Fatal("EntryFilePattern should keep only <hash>_0 files")
	}
}
