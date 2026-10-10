package testfakes

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/representation"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.RepresentationIndex = (*FakeEventStore)(nil)

// SaveRepresentation keeps rep for owner, replacing the same modality and
// kind; owner must be an entity some stored event refers to.
func (f *FakeEventStore) SaveRepresentation(_ context.Context, owner storage.EntityKey, rep representation.Representation) (int64, error) {
	if f.FailWith != nil {
		return 0, f.FailWith
	}
	if !f.refersTo(owner) {
		return 0, fmt.Errorf("store %s %s of %s %q: no such entity", rep.Modality, rep.Kind, owner.Type, owner.Key)
	}
	for i, stored := range f.Representations[owner] {
		if stored.Modality == rep.Modality && stored.Kind == rep.Kind {
			f.Representations[owner][i].Representation = rep
			return stored.ID, nil
		}
	}
	if f.Representations == nil {
		f.Representations = map[storage.EntityKey][]storage.StoredRepresentation{}
	}
	f.representationCount++
	id := f.representationCount
	f.Representations[owner] = append(f.Representations[owner], storage.StoredRepresentation{ID: id, Representation: rep})
	return id, nil
}

// RepresentationsOf lists owner's representations by modality and kind,
// while some stored event still refers to owner.
func (f *FakeEventStore) RepresentationsOf(_ context.Context, owner storage.EntityKey) ([]storage.StoredRepresentation, error) {
	if !f.refersTo(owner) {
		return nil, f.FailWith
	}
	stored := slices.Clone(f.Representations[owner])
	slices.SortFunc(stored, func(a, b storage.StoredRepresentation) int {
		return strings.Compare(string(a.Modality)+"|"+string(a.Kind), string(b.Modality)+"|"+string(b.Kind))
	})
	return stored, f.FailWith
}

func (f *FakeEventStore) refersTo(owner storage.EntityKey) bool {
	for _, ev := range f.Events {
		if slices.ContainsFunc(entity.RefsOf(ev), func(ref entity.Ref) bool { return ref.Type == owner.Type && ref.Key == owner.Key }) {
			return true
		}
	}
	return false
}
