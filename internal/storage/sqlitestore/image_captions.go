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
