package sqlitestore

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.ImageCaptionIndex = (*Store)(nil)

// describedCandidates bounds the events read for one hash: copies of an
// image share it, and any described one will do.
const describedCandidates = 8

// DescribedImage returns a stored description of the image with this hash:
// the status and size of a described copy (index of schema version 10)
// with the Image's representations.
func (s *Store) DescribedImage(ctx context.Context, sha256 string) (event.Image, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT metadata FROM events WHERE `+imageSHA256Expression+` = ? AND source = ? LIMIT ?`,
		sha256, string(event.SourceFile), describedCandidates)
	if err != nil {
		return event.Image{}, false, fmt.Errorf("find images with hash %q: %w", sha256, err)
	}
	encoded, err := collectStrings(rows)
	if err != nil {
		return event.Image{}, false, fmt.Errorf("read images with hash %q: %w", sha256, err)
	}
	for _, raw := range encoded {
		metadata, err := decodeMetadata(raw)
		if err != nil {
			return event.Image{}, false, err
		}
		if image := (event.Event{Metadata: metadata}).Image(); image.Status == event.CaptionDescribed {
			return s.withImageText(ctx, image)
		}
	}
	return event.Image{}, false, nil
}

// OutdatedImages lists, oldest first, described images present in their
// folder whose Image's description another model or prompt version wrote.
func (s *Store) OutdatedImages(ctx context.Context, model string, promptVersion int) ([]event.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events
		JOIN event_entities ON event_entities.event_id = events.id AND event_entities.role = ?
		JOIN representations ON representations.entity_id = event_entities.entity_id AND representations.kind = ?
		WHERE events.source = ? AND `+eventMetadataField(event.CaptionStatusKey)+` = ? AND coalesce(`+eventMetadataField(event.RemovedAtKey)+`, '') = ''
		AND (representations.model IS NOT ? OR representations.revision IS NOT ?)
		ORDER BY events.occurred_at, events.id`,
		string(entity.RoleShows), string(representation.KindDescription), string(event.SourceFile), string(event.CaptionDescribed), model, promptVersion)
	if err != nil {
		return nil, fmt.Errorf("find image descriptions not by %s with prompt version %d: %w", model, promptVersion, err)
	}
	return collectEvents(rows)
}

// eventMetadataField reads one key of events.metadata, for queries that
// join another table with a metadata column.
func eventMetadataField(key string) string {
	return jsonField("events.metadata", key)
}

// metadataField reads one metadata key in SQL. The image hash index
// (schema version 10) was built on this exact expression, which a query
// must repeat for SQLite to use it.
func metadataField(key string) string {
	return jsonField("metadata", key)
}

func jsonField(column, key string) string {
	return `json_extract(` + column + `, '$.` + key + `')`
}
