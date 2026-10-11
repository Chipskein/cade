package chromiumstorage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/webstore"
)

const (
	chatGPT       = "https://chatgpt.com"
	teams         = "https://teams.cloud.microsoft"
	draftsKey     = "oai/apps/conversationDrafts"
	draftsJSON    = `{"drafts":[{"id":"c1","content":"olá, rascunho","timestamp":1790941431821}]}`
	sessionToken  = "secret-token"
	themeKey      = "theme"
	unicodeText   = "ação ✓"
	localStorages = "Local Storage"
)

func latin1Stored(text string) []byte {
	return append([]byte{formatLatin1}, text...)
}

func utf16Stored(text string) []byte {
	stored := []byte{formatUTF16}
	for _, unit := range utf16.Encode([]rune(text)) {
		stored = append(stored, byte(unit), byte(unit>>8))
	}
	return stored
}

func itemKey(origin string, key []byte) []byte {
	return append(append([]byte("_"+origin), 0), key...)
}

// writeLocalStorage lays out a profile's localStorage: two items of
// ChatGPT, a token of Teams, the metadata keys Chromium keeps beside them.
func writeLocalStorage(t *testing.T) string {
	t.Helper()
	location := filepath.Join(t.TempDir(), localStorages, "leveldb")
	testcheck.NoError(t, os.MkdirAll(location, 0o700))
	var journal testfakes.LevelDBJournal
	journal.Put([]byte("VERSION"), []byte("1"))
	journal.Put([]byte("META:"+chatGPT), []byte{0x08, 0x01})
	journal.Put(itemKey(chatGPT, latin1Stored(draftsKey)), utf16Stored(draftsJSON))
	journal.Put(itemKey(chatGPT, utf16Stored(themeKey)), utf16Stored(unicodeText))
	journal.Put(itemKey(teams, latin1Stored("SKYPE-TOKEN")), latin1Stored(sessionToken))
	testcheck.NoError(t, journal.WriteDir(location))
	return location
}

func readerOf(t *testing.T, origins ...string) LocalStorageReader {
	t.Helper()
	scope, err := sitestorage.NewScope(origins)
	testcheck.NoError(t, err)
	return NewLocalStorageReader(scope)
}

func recordOf(records []webstore.Record, container string) *webstore.Record {
	for i := range records {
		if records[i].Container == container {
			return &records[i]
		}
	}
	return nil
}

func TestReadGivesOnlyTheItemsOfConfiguredOrigins(t *testing.T) {
	records, err := readerOf(t, chatGPT).Read(writeLocalStorage(t))
	testcheck.NoError(t, err)
	if len(records) != 2 {
		t.Fatalf("Read = %d records %+v; want the two ChatGPT items", len(records), records)
	}
	for _, record := range records {
		if record.Origin != chatGPT || record.Kind != webstore.KindLocalStorage || strings.Contains(record.Value.String(), sessionToken) {
			t.Fatalf("record %+v; want only ChatGPT localStorage items", record)
		}
	}
}

func TestReadParsesJSONItemsIntoTrees(t *testing.T) {
	records, err := readerOf(t, chatGPT).Read(writeLocalStorage(t))
	testcheck.NoError(t, err)
	drafts := recordOf(records, draftsKey)
	if drafts == nil || drafts.Key != draftsKey || drafts.Value.Get("drafts").Items[0].Get("content").String() != "olá, rascunho" {
		t.Fatalf("drafts record = %+v; want the parsed draft", drafts)
	}
}

func TestReadKeepsTextItemsAndUTF16Keys(t *testing.T) {
	records, err := readerOf(t, chatGPT).Read(writeLocalStorage(t))
	testcheck.NoError(t, err)
	if theme := recordOf(records, themeKey); theme == nil || theme.Value.Text != unicodeText {
		t.Fatalf("theme record = %+v; want the text %q", theme, unicodeText)
	}
}

func TestReadRefusesAnEmptyScope(t *testing.T) {
	if _, err := readerOf(t).Read(writeLocalStorage(t)); !errors.Is(err, sitestorage.ErrEmptyScope) {
		t.Fatalf("Read error = %v; want ErrEmptyScope", err)
	}
}

func TestReadReportsAnUndecodableValue(t *testing.T) {
	location := filepath.Join(t.TempDir(), localStorages, "leveldb")
	testcheck.NoError(t, os.MkdirAll(location, 0o700))
	var journal testfakes.LevelDBJournal
	journal.Put(itemKey(chatGPT, latin1Stored(draftsKey)), []byte{formatUTF16, 'x'})
	testcheck.NoError(t, journal.WriteDir(location))
	records, err := readerOf(t, chatGPT).Read(location)
	if err != nil || len(records) != 1 || records[0].DecodeErr == nil || !strings.Contains(records[0].DecodeErr.Error(), draftsKey) {
		t.Fatalf("Read = %+v, %v; want one record with a DecodeErr naming the key", records, err)
	}
}

func TestRecognizesOnlyALocalStorageLevelDB(t *testing.T) {
	location := writeLocalStorage(t)
	reader := readerOf(t, chatGPT)
	if !reader.Recognizes(location) || !reader.Recognizes(location+"/") {
		t.Fatalf("Recognizes(%q) = false; want true", location)
	}
	indexedDB := filepath.Join(t.TempDir(), "https_chatgpt.com_0.indexeddb.leveldb")
	testcheck.NoError(t, os.MkdirAll(indexedDB, 0o700))
	testcheck.NoError(t, os.WriteFile(filepath.Join(indexedDB, leveldbCurrentFile), nil, 0o600))
	if reader.Recognizes(indexedDB) || reader.Recognizes(filepath.Dir(location)) {
		t.Fatal("Recognizes = true for an IndexedDB LevelDB or the parent directory")
	}
}

func TestDecodeStorageStringRejectsUnknownFormat(t *testing.T) {
	if _, err := decodeStorageString([]byte{7, 'a'}); err == nil || !strings.Contains(err.Error(), "format 7") {
		t.Fatalf("decodeStorageString error = %v; want the format named", err)
	}
	if _, err := decodeStorageString(nil); err == nil {
		t.Fatal("decodeStorageString(nil) = nil error; want one")
	}
}

func TestReadAnItemLargerThanAJournalBlock(t *testing.T) {
	location := filepath.Join(t.TempDir(), localStorages, "leveldb")
	testcheck.NoError(t, os.MkdirAll(location, 0o700))
	large := strings.Repeat("a", 70_000)
	var journal testfakes.LevelDBJournal
	journal.Put(itemKey(chatGPT, latin1Stored(themeKey)), latin1Stored(large))
	testcheck.NoError(t, journal.WriteDir(location))
	records, err := readerOf(t, chatGPT).Read(location)
	if err != nil || len(records) != 1 || records[0].Value.Text != large {
		t.Fatalf("Read = %d records, %v; want the whole large item", len(records), err)
	}
}
