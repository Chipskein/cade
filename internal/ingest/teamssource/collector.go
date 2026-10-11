// Package teamssource ingests Microsoft Teams chat messages from the
// IndexedDB the Teams web client keeps in a Chromium or Firefox profile
// (RF1.4, best effort: only what the client has cached is available).
package teamssource

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
)

// ReadIndexedDB reads every record of an IndexedDB directory; injected so
// tests need no browser profile.
type ReadIndexedDB func(dir string) ([]indexeddb.Record, error)

// Collector reads one Teams IndexedDB directory.
type Collector struct {
	read     ReadIndexedDB
	dir      string
	captions ingest.ImageCaptions
}

// NewCollector reads dir (e.g. .../IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb).
//
//	collector := teamssource.NewCollector(indexeddb.ReadDirectory, dir)
func NewCollector(read ReadIndexedDB, dir string) *Collector {
	return &Collector{read: read, dir: dir}
}

// WithCaptions gives the messages the pictures planned for them, by
// address (imagecaption.Planner.PlanImageBytes).
func (c *Collector) WithCaptions(captions ingest.ImageCaptions) *Collector {
	c.captions = captions
	return c
}

// ImageURLs are the addresses of the pictures the cached messages show,
// for planning their descriptions before the messages are collected.
func (c *Collector) ImageURLs(ctx context.Context) ([]string, error) {
	var urls []string
	err := c.CollectEvents(ctx, func(ev event.Event) error {
		urls = append(urls, ev.ImageURLs()...)
		return nil
	})
	return urls, err
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
		origin:        indexeddb.OriginName(c.dir),
		captions:      c.captions,
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
	// A chain can come without messageMap (the fixture's large payload
	// records): it has no messages, and reading its properties panicked.
	messageMap := record.Value.Get("messageMap")
	if messageMap == nil {
		return nil
	}
	for _, entry := range messageMap.Properties {
		tally.countMessage(entry.Value)
		message, ok := parseMessage(entry.Value, messages.senders)
		if !ok {
			continue
		}
		ev := message.toEvent(conversationFor(messages, message.conversationID), messages.origin)
		if err := emit(withPlannedImages(ev, messages.captions)); err != nil {
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
