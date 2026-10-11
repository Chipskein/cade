package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
)

// An image's description and visible text are representations of its
// Image entity (#22): the event keeps the status and the hash
// (event.WithoutImageText), and its text keeps the description, so the
// search is unchanged.

func imageOwner(sha256 string) storage.EntityKey {
	return storage.EntityKey{Type: entity.TypeImage, Key: sha256}
}

// representImage stores the description and visible text of a described
// image event; an image described again without visible text loses the
// old one. Its entities are already linked.
func representImage(ctx context.Context, tx *sql.Tx, ev event.Event) error {
	image := ev.Image()
	if image.SHA256 == "" || image.Status != event.CaptionDescribed {
		return nil
	}
	owner := imageOwner(image.SHA256)
	if _, err := saveRepresentation(ctx, tx, owner, imageText(image, representation.KindDescription, image.Description)); err != nil {
		return err
	}
	if image.VisibleText == "" {
		return deleteRepresentation(ctx, tx, owner, representation.KindVisibleText)
	}
	_, err := saveRepresentation(ctx, tx, owner, imageText(image, representation.KindVisibleText, image.VisibleText))
	return err
}

func imageText(image event.Image, kind representation.Kind, content string) representation.Representation {
	return representation.Representation{Modality: representation.ModalityText, Kind: kind, Model: image.Model, Revision: image.PromptVersion, Content: content}
}

func deleteRepresentation(ctx context.Context, tx *sql.Tx, owner storage.EntityKey, kind representation.Kind) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM representations WHERE kind = ? AND entity_id = (SELECT id FROM entities WHERE type = ? AND entity_key = ?)`,
		string(kind), string(owner.Type), owner.Key)
	if err != nil {
		return fmt.Errorf("delete the %s of %s %q: %w", kind, owner.Type, owner.Key, err)
	}
	return nil
}

// withImageText puts the image's representations back into a stored
// image's metadata, as event.Image reads them; found is false without a
// description.
func (s *Store) withImageText(ctx context.Context, image event.Image) (event.Image, bool, error) {
	stored, err := s.RepresentationsOf(ctx, imageOwner(image.SHA256))
	if err != nil {
		return event.Image{}, false, err
	}
	found := false
	for _, rep := range stored {
		switch rep.Kind {
		case representation.KindDescription:
			image.Description, image.Model, image.PromptVersion, found = rep.Content, rep.Model, rep.Revision, true
		case representation.KindVisibleText:
			image.VisibleText = rep.Content
		}
	}
	return image, found, nil
}
