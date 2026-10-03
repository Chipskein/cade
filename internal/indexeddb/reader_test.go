package indexeddb

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/leveldbraw"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// Real Chrome IndexedDB with synthetic data; regenerate with
// testdata/chrome-indexeddb-pages/generate.sh.
const fixtureDir = "../../testdata/chrome-indexeddb.leveldb"

func fixtureRecords(t *testing.T) []Record {
	t.Helper()
	records, err := ReadDirectory(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return records
}

func recordWithID(records []Record, id string) *v8value.Value {
	for _, record := range records {
		if record.Value.Get("id").String() == id {
			return record.Value
		}
	}
	return nil
}

func TestReadDirectoryNamesDatabasesAndStores(t *testing.T) {
	counts := map[string]int{}
	for _, record := range fixtureRecords(t) {
		counts[record.Database+"/"+record.Store]++
	}
	prefix := "Teams:replychain-manager:fixture/"
	if counts[prefix+"replychains"] != 4 || counts[prefix+"people"] != 1 || counts[prefix+"bulk"] != 320 {
		t.Fatalf("unexpected record counts %v", counts)
	}
}

func TestReadDirectoryRecordsKindOriginAndContainer(t *testing.T) {
	record := fixtureRecords(t)[0]
	if record.Kind != webstore.KindIndexedDB || record.Origin != "chrome-indexeddb.leveldb" || record.Namespace != "Teams:replychain-manager:fixture" || record.Container == "" {
		t.Fatalf("record %+v; want an indexeddb record of origin chrome-indexeddb.leveldb in a named store", record)
	}
}

func TestReadDirectoryDecodesUnicodeMessages(t *testing.T) {
	chain := recordWithID(fixtureRecords(t), "19:abc@thread.v2")
	message := chain.Get("messageMap").Get("1727280000000")
	content := message.Get("content").String()
	if !strings.Contains(content, "Olá, revisão do <b>PR</b> às 15h? ção ✓") || message.Get("emoji").String() != "🎉" {
		t.Fatalf("unexpected content %q / emoji %q", content, message.Get("emoji").String())
	}
}

func TestReadDirectoryUnwrapsSnappyValues(t *testing.T) {
	payload := recordWithID(fixtureRecords(t), "19:big@thread.v2").Get("payload").String()
	if len(payload) != 200003 || !strings.HasSuffix(payload, "fim") {
		t.Fatalf("expected the 200003-char payload ending in fim, got %d chars", len(payload))
	}
}

func TestReadDirectoryHonoursLaterSession(t *testing.T) {
	records := fixtureRecords(t)
	late := recordWithID(records, "19:late@thread.v2")
	for _, record := range records {
		if record.Store == "people" && record.Value.Get("displayName").String() != "Bruno" {
			t.Fatalf("deleted person must be gone, found %q", record.Value.Get("displayName").String())
		}
	}
	if late.Get("note").String() != "gravado na segunda sessão" {
		t.Fatalf("expected the late record, got %+v", late)
	}
}

func TestReadDirectoryMissing(t *testing.T) {
	if _, err := ReadDirectory(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestDecodeRecordsReportsUndecodableValues(t *testing.T) {
	entries := []leveldbraw.Entry{{Key: []byte{0x00, 1, 1, 1, 0x01}, Value: []byte{0x02, 0x00}}}
	records := decodeRecords(entries)
	if len(records) != 1 || records[0].DecodeErr == nil || records[0].Database != "#1" {
		t.Fatalf("expected one failed record under #1, got %+v", records)
	}
}

func TestDecodeRecordsSkipsIndexAndMetadataKeys(t *testing.T) {
	entries := []leveldbraw.Entry{
		{Key: []byte{0x00, 1, 1, 2}, Value: []byte{1}},
		{Key: []byte{0x00, 1, 0, 0, 3}, Value: []byte{1}},
	}
	if records := decodeRecords(entries); len(records) != 0 {
		t.Fatalf("expected no records, got %+v", records)
	}
}

func TestNameOrID(t *testing.T) {
	if nameOrID("chats", 3) != "chats" || nameOrID("", 3) != "#3" {
		t.Fatal("expected the name, or #id when missing")
	}
}

func TestV8PayloadBlobWrapped(t *testing.T) {
	if _, err := v8Payload([]byte{0x02, 0xFF, 0x11, 0x01, 0x10, 0x00}); !errors.Is(err, ErrBlobWrapped) {
		t.Fatalf("expected ErrBlobWrapped, got %v", err)
	}
}

func TestStripBlinkHeaderWithTrailer(t *testing.T) {
	serialized := append([]byte{0xFF, 0x15, 0xFE}, make([]byte, 12)...)
	payload, err := stripBlinkHeader(append(serialized, 0xFF, 0x10, 'T'))
	if err != nil || string(payload) != "\xff\x10T" {
		t.Fatalf("expected the V8 payload, got %x (err %v)", payload, err)
	}
}

func TestStripBlinkHeaderWithoutTrailer(t *testing.T) {
	payload, err := stripBlinkHeader([]byte{0xFF, 0x13, 0xFF, 0x0F, 'F'})
	if err != nil || string(payload) != "\xff\x0fF" {
		t.Fatalf("expected the V8 payload, got %x (err %v)", payload, err)
	}
}

func TestStripBlinkHeaderRejectsMissingHeader(t *testing.T) {
	if _, err := stripBlinkHeader([]byte{'o'}); err == nil {
		t.Fatal("expected an error without the Blink header byte")
	}
}

func TestStripBlinkHeaderRejectsShortTrailer(t *testing.T) {
	if _, err := stripBlinkHeader([]byte{0xFF, 0x15, 0xFE, 0x00}); err == nil {
		t.Fatal("expected an error for a truncated trailer")
	}
}
