package storage

import "testing"

func TestVectorSlotsEmptyShare(t *testing.T) {
	cases := map[VectorSlots]float64{
		{}:                           0,
		{Slots: 1024, Vectors: 1024}: 0,
		{Slots: 2048, Vectors: 1024}: 0.5,
		{Slots: 1024}:                1,
	}
	for slots, want := range cases {
		if got := slots.EmptyShare(); got != want {
			t.Errorf("%+v.EmptyShare() = %v, want %v", slots, got, want)
		}
	}
}
