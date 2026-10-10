package ingest

import (
	"context"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// StoredEventsReader is what a PairingCollector may read from the store.
type StoredEventsReader interface {
	EventsMatching(ctx context.Context, filter storage.EventFilter) ([]event.Event, error)
}

// PairingCollector completes events another source already stored (#77:
// the Android database fills in the text of WhatsApp Web messages), so it
// reads them first. The pipeline calls CollectPaired instead of
// CollectEvents; the collector must finish reading before its first emit,
// since that opens the write batch (#51).
type PairingCollector interface {
	EventCollector
	CollectPaired(ctx context.Context, stored StoredEventsReader, emit EmitFunc) error
}

// collectFrom runs the collector, giving a PairingCollector the store.
func (p *Pipeline) collectFrom(ctx context.Context, collector EventCollector, emit EmitFunc) error {
	if pairing, pairs := collector.(PairingCollector); pairs {
		return pairing.CollectPaired(ctx, p.store, emit)
	}
	return collector.CollectEvents(ctx, emit)
}

// keepsBackupText reports a stored message whose text came from the
// Android database and an incoming reading without it: the WhatsApp Web
// schema has no revision, so without this every `ingest whatsapp` would
// erase the text the backup filled in.
func keepsBackupText(incoming, stored event.Event) bool {
	return stored.TextFromBackup() && incoming.Message().Text == ""
}
