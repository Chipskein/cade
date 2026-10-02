package firefoxidb

import (
	"bytes"
	"errors"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/smclone"
	"github.com/chipskein/cade/internal/v8value"
)

// Real Floorp IndexedDB with synthetic data; regenerate with
// testdata/firefox-indexeddb-pages/generate.sh.
const sampleDir = "../../testdata/firefox-indexeddb"

func sampleRecords(t *testing.T) []indexeddb.Record {
	t.Helper()
	records, err := NewReader(smclone.Decode, openForTest).ReadDirectory(sampleDir)
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	return records
}

func sampleMessage(t *testing.T) *v8value.Value {
	t.Helper()
	for _, record := range sampleRecords(t) {
		if record.Store == "message" && record.DecodeErr == nil {
			return record.Value
		}
	}
	t.Fatal("no decoded message in the sample")
	return nil
}

func TestSampleNamesDatabaseAndStores(t *testing.T) {
	counts := map[string]int{}
	for _, record := range sampleRecords(t) {
		counts[record.Database+"/"+record.Store]++
	}
	if counts["model-storage-fixture/message"] != 2 || counts["model-storage-fixture/contact"] != 1 {
		t.Fatalf("unexpected record counts %v", counts)
	}
}

func TestSampleDecodesTextInBothEncodings(t *testing.T) {
	message := sampleMessage(t)
	if body := message.Get("body").String(); body != "Olá, revisão do PR às 15h? ção ✓" || message.Get("emoji").String() != "🎉" {
		t.Fatalf("unexpected body %q / emoji %q", body, message.Get("emoji").String())
	}
}

func TestSampleDecodesNumbersDatesAndBigInts(t *testing.T) {
	message := sampleMessage(t)
	when := message.Get("when")
	if message.Get("t").Number != 1727280000 || message.Get("negative").Number != -7 || message.Get("ratio").Number != 0.5 {
		t.Fatalf("unexpected numbers t=%v negative=%v ratio=%v", message.Get("t"), message.Get("negative"), message.Get("ratio"))
	}
	if when.Kind != v8value.KindDate || when.Number != 1790348400000 || message.Get("big").Text != "12345678901234567890" {
		t.Fatalf("unexpected date %+v / bigint %+v", when, message.Get("big"))
	}
}

func TestSampleDecodesContainersAndSharedObjects(t *testing.T) {
	message := sampleMessage(t)
	sparse, lookup := message.Get("sparse"), message.Get("lookup")
	if len(sparse.Items) != 6 || sparse.Items[5].Text != "x" || sparse.Items[0] != nil {
		t.Fatalf("unexpected sparse array %+v", sparse.Items)
	}
	if len(lookup.Entries) != 2 || lookup.Entries[1].Value.Text != "v" || len(message.Get("uniq").Items) != 2 {
		t.Fatalf("unexpected map %+v / set %+v", lookup.Entries, message.Get("uniq"))
	}
	if message.Get("again") != message.Get("props") || message.Get("author").Get("_serialized").String() != "5500000000002@c.us" {
		t.Fatal("expected the shared object to be decoded once and referenced twice")
	}
}

func TestSampleDecodesBinaryRegExpAndBlob(t *testing.T) {
	message := sampleMessage(t)
	if !bytes.Equal(message.Get("view").Bytes, []byte{3, 4, 5, 6}) || len(message.Get("buffer").Bytes) != 8 {
		t.Fatalf("unexpected view %v / buffer %v", message.Get("view").Bytes, message.Get("buffer").Bytes)
	}
	if message.Get("pattern").Text != "ab+c" {
		t.Fatalf("unexpected regexp %+v", message.Get("pattern"))
	}
	attachment := message.Get("attachment")
	if attachment.Get("type").String() != "text/plain" || attachment.Get("size").Number != float64(len("conteúdo")) {
		t.Fatalf("unexpected blob %+v", attachment.Properties)
	}
}

func TestSampleReportsTheExternalCloneAsBlobWrapped(t *testing.T) {
	wrapped := 0
	for _, record := range sampleRecords(t) {
		if errors.Is(record.DecodeErr, indexeddb.ErrBlobWrapped) {
			wrapped++
		}
	}
	if wrapped != 1 {
		t.Fatalf("expected the big message to be blob wrapped, got %d", wrapped)
	}
}
