package webstore_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testfakes"
	"github.com/chipskein/cade/internal/webstore"
)

const (
	chromiumLayout = ".indexeddb.leveldb"
	firefoxLayout  = "/idb"
	storageLayout  = "/ls"
)

func browserReaders() (webstore.Readers, *testfakes.FakeStoreReader, *testfakes.FakeStoreReader) {
	chromium := &testfakes.FakeStoreReader{StoreKind: webstore.KindIndexedDB, Suffix: chromiumLayout}
	firefox := &testfakes.FakeStoreReader{StoreKind: webstore.KindIndexedDB, Suffix: firefoxLayout}
	local := &testfakes.FakeStoreReader{StoreKind: webstore.KindLocalStorage, Suffix: storageLayout}
	return webstore.Readers{chromium, local, firefox}, firefox, local
}

func TestDetectPicksTheReaderOfTheLayout(t *testing.T) {
	readers, firefox, _ := browserReaders()
	reader, err := readers.Detect("/p/storage/default/https+++web.whatsapp.com/idb")
	if err != nil || reader != firefox {
		t.Fatalf("Detect = %v, %v; want the Firefox reader", reader, err)
	}
}

func TestDetectNamesTheLocationAndKindsItKnows(t *testing.T) {
	readers, _, _ := browserReaders()
	_, err := readers.Detect("/p/cache2")
	if err == nil || !strings.Contains(err.Error(), `"/p/cache2"`) || !strings.Contains(err.Error(), "indexeddb, local_storage") {
		t.Fatalf("Detect error = %v; want the location and each kind once", err)
	}
}

func TestReadKindReadsWithTheReaderOfThatKind(t *testing.T) {
	readers, _, local := browserReaders()
	local.Records = []webstore.Record{{Kind: webstore.KindLocalStorage, Container: "chat"}}
	records, err := readers.ReadKind(webstore.KindLocalStorage, "/p/ls")
	if err != nil || len(records) != 1 || len(local.ReadLocations) != 1 {
		t.Fatalf("ReadKind = %v, %v (reads %v); want the localStorage record", records, err, local.ReadLocations)
	}
}

func TestReadKindRefusesALocationOfAnotherLayout(t *testing.T) {
	readers, _, local := browserReaders()
	_, err := readers.ReadKind(webstore.KindLocalStorage, "/p/https+++web.whatsapp.com/idb")
	if err == nil || !strings.Contains(err.Error(), "local_storage") || len(local.ReadLocations) != 0 {
		t.Fatalf("ReadKind error = %v; want a refusal naming the kind, without reading", err)
	}
}

func TestReadKindPassesTheReadersError(t *testing.T) {
	readers, firefox, _ := browserReaders()
	firefox.Err = errors.New("locked")
	if _, err := readers.ReadKind(webstore.KindIndexedDB, "/p/idb"); !errors.Is(err, firefox.Err) {
		t.Fatalf("ReadKind error = %v; want the reader's", err)
	}
}

func TestInContainerSelectsByNamespacePrefixAndContainer(t *testing.T) {
	record := webstore.Record{Namespace: "Teams:replychain-manager:u1", Container: "replychains"}
	if !record.InContainer("Teams:replychain-manager:", "replychains") {
		t.Fatal("InContainer = false; want the prefix to match")
	}
	if record.InContainer("Teams:replychain-manager:", "profiles") {
		t.Fatal("InContainer = true for another container")
	}
}

func TestInContainerLeavesOutUndecodedRecords(t *testing.T) {
	record := webstore.Record{Container: "chat", DecodeErr: webstore.ErrExternalValue}
	if record.InContainer("", "chat") {
		t.Fatal("InContainer = true for a record that did not decode")
	}
}

func TestValidateAcceptsKnownKindsOnly(t *testing.T) {
	if err := webstore.KindHTTPCache.Validate(); err != nil {
		t.Fatalf("Validate(http_cache) = %v", err)
	}
	err := webstore.Kind("indexed_db").Validate()
	if err == nil || !strings.Contains(err.Error(), `"indexed_db"`) || !strings.Contains(err.Error(), "cache_api") {
		t.Fatalf("Validate(indexed_db) = %v; want the value and the kinds expected", err)
	}
}
