package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

// pageSize bounds how many events the export holds at once.
const pageSize = 500

// Store is what the export reads.
type Store interface {
	storage.EventPager
	EntitiesOfEvent(ctx context.Context, uid string) ([]storage.EntityLink, error)
	RelationsStatedBy(ctx context.Context, uid string) ([]storage.Relationship, error)
	RepresentationsOf(ctx context.Context, owner storage.EntityKey) ([]storage.StoredRepresentation, error)
}

// Write writes one Record per event filter keeps, in the order they were
// stored, and returns how many it wrote.
//
//	count, err := export.Write(ctx, store, os.Stdout, storage.EventFilter{To: tomorrow})
func Write(ctx context.Context, store Store, out io.Writer, filter storage.EventFilter) (int, error) {
	encoder := json.NewEncoder(out)
	written := 0
	for cursor := storage.EventCursor(0); ; {
		page, err := store.EventsAfter(ctx, filter, cursor, pageSize)
		if err != nil {
			return written, err
		}
		if err := writePage(ctx, store, encoder, page.Events); err != nil {
			return written, err
		}
		written, cursor = written+len(page.Events), page.Next
		if page.Done {
			return written, nil
		}
	}
}

func writePage(ctx context.Context, store Store, encoder *json.Encoder, events []event.Event) error {
	for _, ev := range events {
		record, err := recordOf(ctx, store, ev)
		if err != nil {
			return err
		}
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("write event %q: %w", ev.UID, err)
		}
	}
	return nil
}

// recordOf is ev with what was derived from it.
func recordOf(ctx context.Context, store Store, ev event.Event) (Record, error) {
	record := Record{UID: ev.UID, Source: ev.Source, OccurredAt: ev.Timestamp, Content: ev.Content, Metadata: ev.Metadata, People: peopleOf(ev)}
	links, err := store.EntitiesOfEvent(ctx, ev.UID)
	if err != nil {
		return Record{}, err
	}
	stated, err := store.RelationsStatedBy(ctx, ev.UID)
	if err != nil {
		return Record{}, err
	}
	record.Relations = relationsOf(stated)
	return withEntities(ctx, store, record, links)
}

// withEntities adds the event's entities and their representations.
func withEntities(ctx context.Context, store Store, record Record, links []storage.EntityLink) (Record, error) {
	for _, link := range links {
		record.Entities = append(record.Entities, Entity{EntityKey: keyOf(link.Entity), Name: link.Name, Role: string(link.Role)})
		stored, err := store.RepresentationsOf(ctx, storage.EntityKey{Type: link.Type, Key: link.Key})
		if err != nil {
			return Record{}, err
		}
		record.Representations = append(record.Representations, representationsOf(link.Entity, stored)...)
	}
	return record, nil
}
