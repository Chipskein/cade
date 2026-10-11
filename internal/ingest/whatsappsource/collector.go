// Package whatsappsource ingests the WhatsApp Android client's database
// (msgstore.db), decrypted by the user (#77). A message WhatsApp Web
// already indexed by metadata alone keeps its event and gets the text;
// the others become events keyed as the Web would key them.
package whatsappsource

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/msgstore"
	"github.com/chipskein/cade/internal/storage"
)

// OpenDatabase opens a SQLite file; injected so this package does not pick
// the driver.
type OpenDatabase func(path string) (*sql.DB, error)

// Tally counts how the last collection paired the database's messages.
type Tally struct {
	Paired int
	New    int
}

// Collector reads one msgstore.db.
type Collector struct {
	open  OpenDatabase
	path  string
	tally Tally
}

// NewCollector reads the msgstore.db at path, through a copy.
//
//	collector := whatsappsource.NewCollector(openSQLite, "~/whatsapp/msgstore.db")
func NewCollector(open OpenDatabase, path string) *Collector {
	return &Collector{open: open, path: path}
}

// Tally returns the counts of the last collection.
func (c *Collector) Tally() Tally {
	return c.tally
}

// CollectEvents emits every message as new, pairing none.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	return c.CollectPaired(ctx, noStoredEvents{}, emit)
}

// CollectPaired reads the whole database and the stored WhatsApp events of
// its period before emitting, since the first emit opens the write batch.
func (c *Collector) CollectPaired(ctx context.Context, stored ingest.StoredEventsReader, emit ingest.EmitFunc) error {
	path, err := filepath.Abs(c.path)
	if err != nil {
		return fmt.Errorf("resolve msgstore path %q: %w", c.path, err)
	}
	messages, err := c.readSnapshot(ctx, path)
	if err != nil || len(messages) == 0 {
		return err
	}
	known, err := storedInPeriod(ctx, stored, messages)
	if err != nil {
		return err
	}
	return c.emitAll(ctx, newEventBuilder(path, known), messages, emit)
}

func (c *Collector) emitAll(ctx context.Context, builder eventBuilder, messages []msgstore.Message, emit ingest.EmitFunc) error {
	c.tally = Tally{}
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return err
		}
		ev, paired := builder.build(message)
		c.count(paired)
		if err := emit(ev); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) count(paired bool) {
	if paired {
		c.tally.Paired++
		return
	}
	c.tally.New++
}

// readSnapshot reads a copy of the database: the user's file is never
// opened by the driver, which may write to it.
func (c *Collector) readSnapshot(ctx context.Context, path string) ([]msgstore.Message, error) {
	tempDir, err := os.MkdirTemp("", "cade-msgstore-*")
	if err != nil {
		return nil, fmt.Errorf("create msgstore snapshot directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	snapshot := filepath.Join(tempDir, filepath.Base(path))
	if err := filecopy.File(path, snapshot); err != nil {
		return nil, fmt.Errorf("snapshot msgstore: %w", err)
	}
	return c.readMessages(ctx, snapshot, path)
}

func (c *Collector) readMessages(ctx context.Context, snapshot, path string) ([]msgstore.Message, error) {
	db, err := c.open(snapshot)
	if err != nil {
		return nil, fmt.Errorf("open msgstore snapshot of %q: %w", path, err)
	}
	defer db.Close()
	var messages []msgstore.Message
	err = msgstore.Read(ctx, db, path, func(message msgstore.Message) error {
		messages = append(messages, message)
		return nil
	})
	return messages, err
}

// storedInPeriod reads the stored WhatsApp events between the first and
// the last message (Read returns them oldest first).
func storedInPeriod(ctx context.Context, stored ingest.StoredEventsReader, messages []msgstore.Message) ([]event.Event, error) {
	filter := storage.EventFilter{Source: event.SourceWhatsApp, From: messages[0].SentAt.Add(-pairingMargin), To: messages[len(messages)-1].SentAt.Add(pairingMargin)}
	known, err := stored.EventsMatching(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("read stored WhatsApp events to pair: %w", err)
	}
	return known, nil
}

// pairingMargin widens the period: the Web stores a message's time in
// seconds and its clock may differ from the phone's.
const pairingMargin = time.Hour

// noStoredEvents pairs nothing.
type noStoredEvents struct{}

func (noStoredEvents) EventsMatching(context.Context, storage.EventFilter) ([]event.Event, error) {
	return nil, nil
}
