// Package teamssource ingests Microsoft Teams chat messages from the
// IndexedDB the Teams web client keeps in a Chromium profile (RF1.4, best
// effort: only what the client has cached is available).
package teamssource

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
)

// ReadIndexedDB reads every record of an IndexedDB directory; injected so
// tests need no browser profile.
type ReadIndexedDB func(dir string) ([]indexeddb.Record, error)

// Collector reads one Teams IndexedDB directory.
type Collector struct {
	read ReadIndexedDB
	dir  string
}

// NewCollector reads dir (e.g. .../IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb).
//
//	collector := teamssource.NewCollector(indexeddb.ReadDirectory, dir)
func NewCollector(read ReadIndexedDB, dir string) *Collector {
	return &Collector{read: read, dir: dir}
}

// CollectEvents emits one event per cached conversation message.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	records, err := c.read(c.dir)
	if err != nil {
		return fmt.Errorf("read Teams IndexedDB %q: %w", c.dir, err)
	}
	messages := messageContext{
		conversations: conversationInfos(records),
		senders:       profileNames(records),
		origin:        originName(c.dir),
	}
	tally := formatTally{records: len(records)}
	for _, record := range records {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err := emitReplyChain(record, messages, emit, &tally); err != nil {
			return err
		}
	}
	return tally.check(c.dir)
}

// emitReplyChain emits the messages of one reply chain (a thread root and
// its replies, keyed by message id in messageMap).
func emitReplyChain(record indexeddb.Record, messages messageContext, emit ingest.EmitFunc, tally *formatTally) error {
	if !isStore(record, replyChainDatabasePrefix, replyChainStore) {
		return nil
	}
	tally.replyChains++
	for _, entry := range record.Value.Get("messageMap").Properties {
		tally.countMessage(entry.Value)
		message, ok := parseMessage(entry.Value, messages.senders)
		if !ok {
			continue
		}
		if err := emit(message.toEvent(conversationFor(messages, message.conversationID), messages.origin)); err != nil {
			return err
		}
	}
	return nil
}

func conversationFor(messages messageContext, conversationID string) conversationInfo {
	if info, found := messages.conversations[conversationID]; found {
		return info
	}
	return conversationInfo{kind: event.KindOther}
}

// originName is the directory's base name without the IndexedDB suffix,
// e.g. "https_teams.cloud.microsoft_0".
func originName(dir string) string {
	return strings.TrimSuffix(filepath.Base(filepath.Clean(dir)), ".indexeddb.leveldb")
}
