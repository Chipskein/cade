package sqlitestore

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.ImageCaptionIndex = (*Store)(nil)

// describedCandidates bounds the events read for one hash: copies of an
// image share it, and any described one will do.
const describedCandidates = 8

// DescribedImage returns a stored description of the image with this hash,
// through the index of schema version 10.
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
			return image, true, nil
		}
	}
	return event.Image{}, false, nil
}

// OutdatedImages lists, oldest first, described images present in their
// folder with a description from another model or prompt version.
func (s *Store) OutdatedImages(ctx context.Context, model string, promptVersion int) ([]event.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events
		WHERE source = ? AND `+metadataField(event.CaptionStatusKey)+` = ? AND coalesce(`+metadataField(event.RemovedAtKey)+`, '') = ''
		AND (`+metadataField(event.CaptionModelKey)+` IS NOT ? OR CAST(`+metadataField(event.CaptionPromptVersionKey)+` AS INTEGER) IS NOT ?)
		ORDER BY occurred_at, id`,
		string(event.SourceFile), string(event.CaptionDescribed), model, promptVersion)
	if err != nil {
		return nil, fmt.Errorf("find image descriptions not by %s with prompt version %d: %w", model, promptVersion, err)
	}
	return collectEvents(rows)
}

// metadataField reads one metadata key in SQL.
func metadataField(key string) string {
	return `json_extract(metadata, '$.` + key + `')`
}
