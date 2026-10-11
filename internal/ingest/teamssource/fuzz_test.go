package teamssource

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// FuzzCollectReplyChain feeds any decodable V8 value to the collector as a
// reply chain: whatever shape Teams gives it, collecting fails or skips,
// never panics (a chain without messageMap did). Run longer with `go tool mage fuzz`.
func FuzzCollectReplyChain(f *testing.F) {
	f.Add([]byte{0xFF, 0x0F, 'o', '"', 2, 'i', 'd', '"', 1, 'x', '{', 1})
	f.Add([]byte{0xFF, 0x0F, 'o', '"', 10, 'm', 'e', 's', 's', 'a', 'g', 'e', 'M', 'a', 'p', 'o', '"', 1, '1', '_', '{', 1, '{', 1})
	f.Fuzz(func(t *testing.T, payload []byte) {
		value, err := v8value.Decode(payload)
		if err != nil {
			return
		}
		record := indexeddb.Record{Namespace: testReplyChainDB, Container: replyChainStore, Value: value}
		reader := FakeIndexedDBReader{Records: []indexeddb.Record{record}}
		_ = NewCollector(reader.Read, "/d").CollectEvents(context.Background(), func(event.Event) error { return nil })
	})
}
