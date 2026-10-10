package testfakes

import (
	"cmp"
	"context"
	"fmt"
	"math"
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
			if stored.Content != rep.Content {
				delete(f.RepresentationVectors, stored.ID)
			}
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

// SaveRepresentationVector keeps the vector of a stored representation.
func (f *FakeEventStore) SaveRepresentationVector(_ context.Context, representationID int64, vector []float32) error {
	if _, _, found := f.representationByID(representationID); !found {
		return fmt.Errorf("representation id %d is not stored", representationID)
	}
	if f.RepresentationVectors == nil {
		f.RepresentationVectors = map[int64][]float32{}
	}
	f.RepresentationVectors[representationID] = vector
	return f.FailWith
}

// NearestRepresentations ranks the stored vectors of kind by cosine
// distance.
func (f *FakeEventStore) NearestRepresentations(_ context.Context, vector []float32, kind representation.Kind, limit int) ([]storage.ScoredRepresentation, error) {
	var scored []storage.ScoredRepresentation
	for id, stored := range f.RepresentationVectors {
		rep, owner, found := f.representationByID(id)
		if found && rep.Kind == kind {
			scored = append(scored, storage.ScoredRepresentation{StoredRepresentation: rep, Owner: owner, Distance: cosineDistance(vector, stored)})
		}
	}
	slices.SortFunc(scored, func(a, b storage.ScoredRepresentation) int { return cmp.Compare(a.Distance, b.Distance) })
	return scored[:min(len(scored), limit)], f.FailWith
}

func (f *FakeEventStore) representationByID(id int64) (storage.StoredRepresentation, storage.EntityKey, bool) {
	for owner, stored := range f.Representations {
		for _, rep := range stored {
			if rep.ID == id {
				return rep, owner, true
			}
		}
	}
	return storage.StoredRepresentation{}, storage.EntityKey{}, false
}

func cosineDistance(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range min(len(a), len(b)) {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 1
	}
	return 1 - dot/math.Sqrt(normA*normB)
}
