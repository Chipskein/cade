package chromiumcache

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/simplecache"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/webstore"
	"github.com/klauspost/compress/gzip"
)

const (
	messagesURL  = "https://discordapp.com/api/v9/channels/42/messages?limit=50"
	profileURL   = "https://discordapp.com/api/v9/users/42/profile"
	messagesBody = `[{"id":"7","channel_id":"42","content":"olá"}]`
	scopeName    = "discord-messages"
	// pickleFields stands in for the pickled fields before the headers.
	pickleFields = "\x28\x09\x00\x00\x03\x6d\x4f\x82\x06\x00\x00\x00"
)

func gzipped(t *testing.T, plain string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	_, _ = writer.Write([]byte(plain))
	testcheck.NoError(t, writer.Close())
	return out.Bytes()
}

func httpEntry(key, encoding string, body []byte) testfakes.SimpleCacheEntry {
	stream0 := pickleFields + "HTTP/1.1 200\x00content-type:application/json\x00content-encoding:" + encoding + "\x00\x00\x01\x02"
	return testfakes.SimpleCacheEntry{Key: key, Stream0: []byte(stream0), Body: body}
}

func writeSimpleCache(t *testing.T, entries map[string]testfakes.SimpleCacheEntry) string {
	t.Helper()
	dir := t.TempDir()
	testcheck.NoError(t, os.Mkdir(filepath.Join(dir, indexDir), 0o755))
	testcheck.NoError(t, os.WriteFile(filepath.Join(dir, "index"), []byte{1}, 0o600))
	for name, entry := range entries {
		testcheck.NoError(t, os.WriteFile(filepath.Join(dir, name), entry.Bytes(), 0o600))
	}
	return dir
}

func discordScope(t *testing.T) requestcache.Scope {
	t.Helper()
	scope, err := requestcache.NewScope(map[string][]string{scopeName: {"https://discordapp.com/api/v*/channels/*/messages*"}})
	testcheck.NoError(t, err)
	return scope
}

func TestHTTPReadReturnsTheResponsesInScope(t *testing.T) {
	dir := writeSimpleCache(t, map[string]testfakes.SimpleCacheEntry{
		"0d1c857aa3989262_0": httpEntry("1/0/_dk_https://discordapp.com https://discordapp.com "+messagesURL, "gzip", gzipped(t, messagesBody)),
		"1111111111111111_0": httpEntry("1/0/"+profileURL, "", []byte(`{"bio":"x"}`)),
		"0d1c857aa3989262_s": httpEntry("1/0/"+messagesURL, "", []byte("sparse")),
	})
	records, err := NewHTTPCacheReader(discordScope(t)).Read(dir)
	if err != nil || len(records) != 1 {
		t.Fatalf("Read = %+v, %v; want only the messages response", records, err)
	}
	record := records[0]
	if record.Kind != webstore.KindHTTPCache || record.Container != scopeName || record.Key != messagesURL || record.Origin != "https://discordapp.com" {
		t.Fatalf("record = %+v; want the scope name, URL and origin", record)
	}
	if record.DecodeErr != nil || record.Value.Items[0].Get("content").String() != "olá" {
		t.Fatalf("record value = %+v, %v; want the decoded message", record.Value, record.DecodeErr)
	}
}

// RecordingReaderAt is an entry file that remembers the furthest byte read.
type RecordingReaderAt struct {
	content  []byte
	furthest int64
}

func (r *RecordingReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	r.furthest = max(r.furthest, offset+int64(len(p)))
	return copy(p, r.content[offset:]), nil
}

func TestHTTPReadNeverReadsTheBodyOfAURLOutOfScope(t *testing.T) {
	key := "1/0/" + profileURL
	file := &RecordingReaderAt{content: httpEntry(key, "", []byte(`{"bio":"x"}`)).Bytes()}
	entry, err := simplecache.OpenEntry(file, int64(len(file.content)))
	testcheck.NoError(t, err)
	_, inScope := NewHTTPCacheReader(discordScope(t)).readEntry(entry)
	if keyEnd := int64(simplecache.HeaderSize + len(key)); inScope || file.furthest > keyEnd {
		t.Fatalf("read up to %d (key ends at %d), in scope %v; want only the key read", file.furthest, keyEnd, inScope)
	}
}

func TestHTTPReadSkipsEntriesItCannotOpen(t *testing.T) {
	dir := writeSimpleCache(t, map[string]testfakes.SimpleCacheEntry{"0d1c857aa3989262_0": httpEntry("1/0/"+messagesURL, "", []byte(messagesBody))})
	testcheck.NoError(t, os.WriteFile(filepath.Join(dir, "2222222222222222_0"), []byte("truncated"), 0o600))
	records, err := NewHTTPCacheReader(discordScope(t)).Read(dir)
	if err != nil || len(records) != 1 {
		t.Fatalf("Read = %d records, %v; want the good entry and no error", len(records), err)
	}
}

func TestHTTPReadRefusesAnEmptyScope(t *testing.T) {
	if _, err := NewHTTPCacheReader(requestcache.Scope{}).Read(writeSimpleCache(t, nil)); !errors.Is(err, requestcache.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestRecognizesASimpleCacheDirectory(t *testing.T) {
	reader := NewHTTPCacheReader(discordScope(t))
	if !reader.Recognizes(writeSimpleCache(t, nil)) || reader.Recognizes(t.TempDir()) {
		t.Fatal("Recognizes should accept a simple cache and refuse other directories")
	}
}

func TestRawHeaderLinesStopAtTheEndOfTheHeaders(t *testing.T) {
	lines := rawHeaderLines([]byte(pickleFields + "HTTP/1.1 200\x00a:1\x00\x00HTTP/x"))
	if len(lines) != 2 || lines[0] != "HTTP/1.1 200" || lines[1] != "a:1" {
		t.Fatalf("rawHeaderLines = %q; want the status line and one header", lines)
	}
	if rawHeaderLines([]byte("no headers")) != nil {
		t.Fatal("rawHeaderLines found headers in a stream without a status line")
	}
}
