package testfakes

import (
	"context"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func TestFakeStoreFindsADescribedPageByHash(t *testing.T) {
	store := NewFakeEventStore()
	page := event.PDFPageImage{Number: 3, Image: event.Image{SHA256: "p3", Description: "um contrato"}}
	undescribed := event.PDFPageImage{Number: 4, Image: event.Image{SHA256: "p4"}}
	store.Events = append(store.Events, event.Event{Metadata: event.PDFPageImagesMetadata([]event.PDFPageImage{page, undescribed})})
	if image, found, err := store.DescribedImage(context.Background(), "p3"); err != nil || !found || image.Description != "um contrato" {
		t.Errorf("p3 = %+v, %v, %v", image, found, err)
	}
	if _, found, _ := store.DescribedImage(context.Background(), "p4"); found {
		t.Error("a page without a description is not described")
	}
}
