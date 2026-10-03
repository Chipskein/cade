package firefoxcache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/gzip"
)

const (
	messagesURL  = "https://discord.com/api/v9/channels/42/messages?limit=50"
	profileURL   = "https://discord.com/api/v9/users/@me/profile"
	messagesBody = `[{"id":"7","channel_id":"42","content":"olá"}]`
	// partitionedKey is how Firefox keys a request with state partitioning.
	partitionedKey = "O^partitionKey=%28https%2Cdiscord.com%29,a,:"
	scopeName      = "discord-messages"
)

// syntheticEntry is one cache2 entry before it is laid out on disk.
type syntheticEntry struct {
	key          string
	responseHead string
	body         []byte
}

// bytes lays the entry out as Firefox does: body, metadata, offset.
func (e syntheticEntry) bytes() []byte {
	var out bytes.Buffer
	out.Write(e.body)
	writeUint32 := func(value uint32) { _ = binary.Write(&out, binary.BigEndian, value) }
	writeUint32(0xcafe) // metadata hash, not checked
	for range (len(e.body) + chunkSize - 1) / chunkSize {
		out.Write([]byte{0, 0})
	}
	for _, field := range []uint32{4, 1, 0, 0, 0, 0, uint32(len(e.key)), 0} {
		writeUint32(field)
	}
	out.WriteString(e.key + elementSeparator)
	out.WriteString("request-method\x00GET\x00" + responseHead + "\x00" + e.responseHead + "\x00")
	writeUint32(uint32(len(e.body)))
	return out.Bytes()
}

func gzipped(t *testing.T, plain string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	_, _ = writer.Write([]byte(plain))
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	return out.Bytes()
}

func messagesEntry(t *testing.T) syntheticEntry {
	head := "HTTP/2 200 \r\ncontent-type: application/json\r\nContent-Encoding: gzip\r\n"
	return syntheticEntry{key: partitionedKey + messagesURL, responseHead: head, body: gzipped(t, messagesBody)}
}

func writeCache2(t *testing.T, entries map[string]syntheticEntry) string {
	t.Helper()
	location := filepath.Join(t.TempDir(), "cache2")
	if err := os.MkdirAll(filepath.Join(location, entriesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, entry := range entries {
		if err := os.WriteFile(filepath.Join(location, entriesDir, name), entry.bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return location
}

func discordScope(t *testing.T) requestcache.Scope {
	t.Helper()
	scope, err := requestcache.NewScope(map[string][]string{scopeName: {"https://discord.com/api/v*/channels/*/messages*"}})
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestReadReturnsTheResponsesInScope(t *testing.T) {
	location := writeCache2(t, map[string]syntheticEntry{
		"A1": messagesEntry(t),
		"B2": {key: ":" + profileURL, responseHead: "HTTP/2 200 \r\n", body: []byte(`{"bio":"x"}`)},
	})
	records, err := NewHTTPCacheReader(discordScope(t)).Read(location)
	if err != nil || len(records) != 1 {
		t.Fatalf("Read = %+v, %v; want only the messages response", records, err)
	}
	record := records[0]
	if record.Kind != webstore.KindHTTPCache || record.Container != scopeName || record.Key != messagesURL || record.Origin != "https://discord.com" {
		t.Fatalf("record = %+v; want the scope name, URL and origin", record)
	}
	if record.DecodeErr != nil || record.Value.Items[0].Get("content").String() != "olá" {
		t.Fatalf("record value = %+v, %v; want the decoded message", record.Value, record.DecodeErr)
	}
}

// RecordingReaderAt is an entry file that remembers the lowest offset read.
type RecordingReaderAt struct {
	content    []byte
	lowestRead int64
}

func (r *RecordingReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	r.lowestRead = min(r.lowestRead, offset)
	return copy(p, r.content[offset:]), nil
}

func TestReadNeverReadsTheBodyOfAURLOutOfScope(t *testing.T) {
	entry := syntheticEntry{key: ":" + profileURL, responseHead: "HTTP/2 200 \r\n", body: []byte(`{"bio":"x"}`)}
	file := &RecordingReaderAt{content: entry.bytes(), lowestRead: int64(len(entry.bytes()))}
	_, inScope, err := readEntry(file, int64(len(file.content)), discordScope(t))
	if err != nil || inScope || file.lowestRead < int64(len(entry.body)) {
		t.Fatalf("readEntry read from %d (body ends at %d), in scope %v, %v; want only the metadata read", file.lowestRead, len(entry.body), inScope, err)
	}
}

func TestReadSkipsEntriesItCannotParse(t *testing.T) {
	location := writeCache2(t, map[string]syntheticEntry{"A1": messagesEntry(t)})
	if err := os.WriteFile(filepath.Join(location, entriesDir, "C3"), []byte{0, 0, 9, 9}, 0o600); err != nil {
		t.Fatal(err)
	}
	records, err := NewHTTPCacheReader(discordScope(t)).Read(location)
	if err != nil || len(records) != 1 {
		t.Fatalf("Read = %d records, %v; want the good entry and no error", len(records), err)
	}
}

func TestReadRefusesAnEmptyScope(t *testing.T) {
	location := writeCache2(t, map[string]syntheticEntry{"A1": messagesEntry(t)})
	if _, err := NewHTTPCacheReader(requestcache.Scope{}).Read(location); !errors.Is(err, requestcache.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestParseMetadataRefusesAnOldVersion(t *testing.T) {
	raw := messagesEntry(t).bytes()
	bodySize := int64(binary.BigEndian.Uint32(raw[len(raw)-offsetSize:]))
	metadata := raw[bodySize : len(raw)-offsetSize]
	binary.BigEndian.PutUint32(metadata[metadataHashSize+chunkHashSize:], 1)
	if _, err := parseMetadata(metadata, bodySize); err == nil {
		t.Fatal("parseMetadata accepted version 1")
	}
}

func TestRecognizesACache2Directory(t *testing.T) {
	reader := NewHTTPCacheReader(discordScope(t))
	if !reader.Recognizes(writeCache2(t, nil)) || reader.Recognizes(t.TempDir()) {
		t.Fatal("Recognizes should accept cache2 and refuse other directories")
	}
}
