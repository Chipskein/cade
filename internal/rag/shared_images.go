package rag

import (
	"context"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/storage"
)

// foldSharedImages keeps the best ranked of the hits showing the same
// image, counting the others as its repeats: a WhatsApp photo and the
// file it was saved as are one picture, and listing both wasted a slot
// (#24). Hits arrive best first.
func (a *Answerer) foldSharedImages(ctx context.Context, hits []storage.ScoredEvent) ([]storage.ScoredEvent, error) {
	firstShowing := map[int64]int{}
	var kept []storage.ScoredEvent
	for _, hit := range hits {
		image, err := a.shownImage(ctx, hit.Event.UID)
		if err != nil {
			return nil, err
		}
		if index, seen := firstShowing[image]; seen {
			kept[index].Repeats++
			continue
		}
		if image != 0 {
			firstShowing[image] = len(kept)
		}
		kept = append(kept, hit)
	}
	return kept, nil
}

// shownImage is the id of the image the event shows, 0 when none.
func (a *Answerer) shownImage(ctx context.Context, uid string) (int64, error) {
	links, err := a.store.EntitiesOfEvent(ctx, uid)
	if err != nil {
		return 0, err
	}
	for _, link := range links {
		if link.Type == entity.TypeImage && link.Role == entity.RoleShows {
			return link.ID, nil
		}
	}
	return 0, nil
}
