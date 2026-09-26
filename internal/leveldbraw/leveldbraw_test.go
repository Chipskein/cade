package leveldbraw

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture is a real Chrome IndexedDB directory holding only synthetic
// data (see testdata/chrome-indexeddb-pages): one snappy-compressed table
// plus a journal written in a later session.
const fixtureDir = "../../testdata/chrome-indexeddb.leveldb"

func utf16BE(text string) []byte {
	var encoded []byte
	for _, char := range text {
		encoded = append(encoded, byte(char>>8), byte(char))
	}
	return encoded
}

func fixtureValueFor(t *testing.T, keySuffix string) ([]byte, bool) {
	t.Helper()
	entries, err := ReadLatest(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	for _, entry := range entries {
		if bytes.HasSuffix(entry.Key, utf16BE(keySuffix)) {
			return entry.Value, true
		}
	}
	return nil, false
}

func TestReadLatestSeesTableAndJournalRecords(t *testing.T) {
	early, foundEarly := fixtureValueFor(t, "19:abc@thread.v2")
	late, foundLate := fixtureValueFor(t, "19:late@thread.v2")
	if !foundEarly || !foundLate || !bytes.Contains(early, []byte("messageMap")) || len(late) == 0 {
		t.Fatalf("expected records from both sessions, found early=%v late=%v", foundEarly, foundLate)
	}
}

func TestReadLatestHonoursDeletion(t *testing.T) {
	if _, found := fixtureValueFor(t, "8:orgid:0000"); found {
		t.Fatal("a record deleted in the journal must not be returned")
	}
}

func TestReadLatestReadsSnappyTableBlocks(t *testing.T) {
	entries, _ := ReadLatest(fixtureDir)
	bulk := 0
	for _, entry := range entries {
		if bytes.Contains(entry.Value, []byte("lorem ipsum dolor")) {
			bulk++
		}
	}
	if bulk != 320 {
		t.Fatalf("expected the 320 bulk records flushed to the table, got %d", bulk)
	}
}

func TestReadLatestMissingDirectory(t *testing.T) {
	if _, err := ReadLatest(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestReadLatestIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "LOG"), []byte("text log"), 0o600)
	entries, err := ReadLatest(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("expected no entries and no error, got %d (err %v)", len(entries), err)
	}
}

func TestReadLatestRejectsCorruptTable(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "000001.ldb"), bytes.Repeat([]byte{1}, 60), 0o600)
	if _, err := ReadLatest(dir); err == nil || !strings.Contains(err.Error(), "magic") {
		t.Fatalf("expected a magic-number error, got %v", err)
	}
}

func TestVersionSetKeepsHighestSequence(t *testing.T) {
	versions := newVersionSet()
	versions.record([]byte("k"), 5, false, []byte("new"))
	versions.record([]byte("k"), 3, false, []byte("old"))
	entries := versions.liveEntries()
	if len(entries) != 1 || string(entries[0].Value) != "new" {
		t.Fatalf("expected only the newest value, got %+v", entries)
	}
}

func TestVersionSetDropsDeletedKeys(t *testing.T) {
	versions := newVersionSet()
	versions.record([]byte("k"), 1, false, []byte("v"))
	versions.record([]byte("k"), 2, true, nil)
	if entries := versions.liveEntries(); len(entries) != 0 {
		t.Fatalf("expected deleted key to vanish, got %+v", entries)
	}
}

func internalKey(userKey string, sequence uint64, kind byte) []byte {
	return binary.LittleEndian.AppendUint64([]byte(userKey), sequence<<8|uint64(kind))
}

func TestParseInternalKey(t *testing.T) {
	key, sequence, deleted, err := parseInternalKey(internalKey("abc", 42, typeDeletion))
	if err != nil || string(key) != "abc" || sequence != 42 || !deleted {
		t.Fatalf("unexpected parse %q %d %v (err %v)", key, sequence, deleted, err)
	}
}

func TestParseInternalKeyTooShort(t *testing.T) {
	if _, _, _, err := parseInternalKey([]byte{1, 2}); err == nil {
		t.Fatal("expected an error for a key shorter than its trailer")
	}
}

func encodeBatch(sequence uint64, records ...[]byte) []byte {
	batch := binary.LittleEndian.AppendUint64(nil, sequence)
	batch = binary.LittleEndian.AppendUint32(batch, uint32(len(records)))
	for _, record := range records {
		batch = append(batch, record...)
	}
	return batch
}

func putRecord(key, value string) []byte {
	record := binary.AppendUvarint([]byte{typeValue}, uint64(len(key)))
	record = binary.AppendUvarint(append(record, key...), uint64(len(value)))
	return append(record, value...)
}

func deleteRecord(key string) []byte {
	return append(binary.AppendUvarint([]byte{typeDeletion}, uint64(len(key))), key...)
}

func TestDecodeBatchAssignsConsecutiveSequences(t *testing.T) {
	versions := newVersionSet()
	err := decodeBatch(encodeBatch(10, putRecord("a", "1"), deleteRecord("a"), putRecord("b", "2")), versions)
	entries := versions.liveEntries()
	if err != nil || len(entries) != 1 || string(entries[0].Key) != "b" {
		t.Fatalf("expected only b to survive, got %+v (err %v)", entries, err)
	}
}

func TestDecodeBatchRejectsUnknownRecordType(t *testing.T) {
	if err := decodeBatch(encodeBatch(1, []byte{7, 1, 'k'}), newVersionSet()); err == nil {
		t.Fatal("expected an error for record type 7")
	}
}

func TestDecodeBatchRejectsShortHeader(t *testing.T) {
	if err := decodeBatch([]byte{1, 2, 3}, newVersionSet()); err == nil {
		t.Fatal("expected an error for a truncated header")
	}
}

func TestDecodeBatchRejectsOverlongKey(t *testing.T) {
	if err := decodeBatch(encodeBatch(1, []byte{typeValue, 50, 'k'}), newVersionSet()); err == nil {
		t.Fatal("expected an error for a key length past the end")
	}
}

func TestReadLengthPrefixed(t *testing.T) {
	value, rest, err := readLengthPrefixed([]byte{2, 'h', 'i', '!'})
	if err != nil || string(value) != "hi" || string(rest) != "!" {
		t.Fatalf("expected hi / !, got %q %q (err %v)", value, rest, err)
	}
}
