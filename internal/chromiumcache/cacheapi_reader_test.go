package chromiumcache

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/webstore"
)

// lengthDelimited encodes one string or nested message field.
func lengthDelimited(number uint64, value []byte) []byte {
	raw := binary.AppendUvarint(nil, number<<3|2)
	raw = binary.AppendUvarint(raw, uint64(len(value)))
	return append(raw, value...)
}

// cacheIndex encodes index.txt for caches, name → directory.
func cacheIndex(caches [][2]string) []byte {
	var index []byte
	for _, cache := range caches {
		entry := append(lengthDelimited(cacheNameField, []byte(cache[0])), lengthDelimited(cacheDirNameField, []byte(cache[1]))...)
		index = append(index, lengthDelimited(indexCacheField, entry)...)
	}
	return append(index, lengthDelimited(2, []byte("https://discordapp.com"))...)
}

func writeCacheStorage(t *testing.T, index []byte, caches map[string]map[string]testfakes.SimpleCacheEntry) string {
	t.Helper()
	origin := t.TempDir()
	testcheck.NoError(t, os.WriteFile(filepath.Join(origin, cacheIndexFile), index, 0o600))
	for dir, entries := range caches {
		testcheck.NoError(t, os.Mkdir(filepath.Join(origin, dir), 0o755))
		for name, entry := range entries {
			testcheck.NoError(t, os.WriteFile(filepath.Join(origin, dir, name), entry.Bytes(), 0o600))
		}
	}
	return origin
}

// metadataEntry is a Cache API entry: the key is the URL, stream 0 a
// CacheMetadata message the reader does not need, the body decoded.
func metadataEntry(url, body string) testfakes.SimpleCacheEntry {
	return testfakes.SimpleCacheEntry{Key: url, Stream0: lengthDelimited(1, []byte("GET")), Body: []byte(body)}
}

func TestCacheAPIReadReturnsTheResponsesInScope(t *testing.T) {
	origin := writeCacheStorage(t, cacheIndex([][2]string{{"api-v1", "18617ea9"}, {"assets", "55e6c8e2"}}), map[string]map[string]testfakes.SimpleCacheEntry{
		"18617ea9": {"0d1c857aa3989262_0": metadataEntry(messagesURL, messagesBody), "1111111111111111_0": metadataEntry(profileURL, `{}`)},
		"55e6c8e2": {"2222222222222222_0": metadataEntry("https://discordapp.com/app.js", "x")},
	})
	records, err := NewCacheAPIReader(discordScope(t)).Read(origin)
	if err != nil || len(records) != 1 {
		t.Fatalf("Read = %+v, %v; want only the messages response", records, err)
	}
	record := records[0]
	if record.Kind != webstore.KindCacheAPI || record.Namespace != "api-v1" || record.Container != scopeName || record.Key != messagesURL {
		t.Fatalf("record = %+v; want the cache name, scope name and URL", record)
	}
	if record.DecodeErr != nil || record.Value.Items[0].Get("content").String() != "olá" {
		t.Fatalf("record value = %+v, %v; want the decoded message", record.Value, record.DecodeErr)
	}
}

func TestCacheAPIReadRefusesADirectoryOutsideTheOrigin(t *testing.T) {
	origin := writeCacheStorage(t, cacheIndex([][2]string{{"api", "../elsewhere"}}), nil)
	_, err := NewCacheAPIReader(discordScope(t)).Read(origin)
	if err == nil || !strings.Contains(err.Error(), `"../elsewhere"`) {
		t.Fatalf("Read error = %v; want the directory refused by value", err)
	}
}

func TestCacheAPIReadReportsACorruptIndex(t *testing.T) {
	origin := writeCacheStorage(t, []byte{0x0a, 0x7f}, nil)
	if _, err := NewCacheAPIReader(discordScope(t)).Read(origin); err == nil || !strings.Contains(err.Error(), "index.txt") {
		t.Fatalf("Read error = %v; want the index named", err)
	}
}

func TestCacheAPIReadRefusesAnEmptyScope(t *testing.T) {
	origin := writeCacheStorage(t, cacheIndex(nil), nil)
	if _, err := NewCacheAPIReader(requestcache.Scope{}).Read(origin); !errors.Is(err, requestcache.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestCacheAPIRecognizesAnOriginDirectory(t *testing.T) {
	reader := NewCacheAPIReader(discordScope(t))
	if !reader.Recognizes(writeCacheStorage(t, cacheIndex(nil), nil)) || reader.Recognizes(writeSimpleCache(t, nil)) {
		t.Fatal("Recognizes should accept an origin's CacheStorage and refuse an HTTP cache")
	}
}
