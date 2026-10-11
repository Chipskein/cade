package idbmap

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/v8value"
)

func TestCompareSchemasAcceptsAReplacementKeepingUIDs(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	comparison, err := CompareSchemas(current, current, chainRecords())
	if err != nil || comparison.SharedUIDs != 2 || comparison.Overlap() != 1 || !comparison.Accepted() {
		t.Fatalf("expected the same schema accepted, got %+v (err %v)", comparison, err)
	}
}

// A replacement that reads the conversation elsewhere gives every message
// a new UID: indexed messages would come back duplicated.
func TestCompareSchemasRefusesAChangedIdentity(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	next := mustMapper(t, strings.Replace(chainSchema, `"conversation_id": {"paths": ["$.conversationId"]}`, `"conversation_id": {"paths": ["$.creator"]}`, 1)).schema
	comparison, err := CompareSchemas(current, next, chainRecords())
	if err != nil || comparison.NextEvents != 2 || comparison.SharedUIDs != 0 || comparison.Accepted() {
		t.Fatalf("expected the identity change refused, got %+v (err %v)", comparison, err)
	}
}

func TestCompareSchemasRefusesAReplacementThatMapsLess(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	next := mustMapper(t, strings.Replace(chainSchema, `"in": ["Text", "RichText/Html"]`, `"in": ["Text"]`, 1)).schema
	if comparison, _ := CompareSchemas(current, next, chainRecords()); comparison.NextEvents != 1 || comparison.Accepted() {
		t.Fatalf("expected a replacement mapping fewer messages refused, got %+v", comparison)
	}
}

// The app renamed content to body (and wrote the time as text, set back to
// a number here so only the rename differs).
func TestCompareSchemasAfterTheApplicationChanged(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	next := mustMapper(t, strings.Replace(chainSchema, `"paths": ["$.content"]`, `"paths": ["$.body"]`, 1)).schema
	records := renamedRecords()
	for _, message := range records[0].Value.Get("messageMap").Properties {
		message.Value.Get("originalArrivalTime").Kind, message.Value.Get("originalArrivalTime").Number = v8value.KindNumber, 1727280000000
	}
	comparison, err := CompareSchemas(current, next, records)
	if err != nil || !comparison.Accepted() || comparison.Overlap() != 1 {
		t.Fatalf("expected the regenerated schema accepted with the same UIDs, got %+v (err %v)", comparison, err)
	}
}
