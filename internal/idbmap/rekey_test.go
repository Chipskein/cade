package idbmap

import (
	"strings"
	"testing"
)

func TestUIDChangesPairsEachMessage(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	next := mustMapper(t, strings.Replace(chainSchema, `"conversation_id": {"paths": ["$.conversationId"]}`, `"conversation_id": {"paths": ["$.creator"]}`, 1)).schema
	changes, err := UIDChanges(current, next, chainRecords())
	if err != nil || len(changes) != 2 {
		t.Fatalf("expected the two kept messages paired, got %v (err %v)", changes, err)
	}
	for oldUID, newUID := range changes {
		if oldUID == newUID {
			t.Fatalf("expected different UIDs, got %q -> %q", oldUID, newUID)
		}
	}
}

func TestUIDChangesIsEmptyWhenIdentityHolds(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	next := mustMapper(t, strings.Replace(chainSchema, `"transform": "html_text"`, `"transform": "trim"`, 1)).schema
	if changes, err := UIDChanges(current, next, chainRecords()); err != nil || len(changes) != 0 {
		t.Fatalf("expected no change when only the text is read differently, got %v (err %v)", changes, err)
	}
}

func TestUIDChangesNeedsTheSameItems(t *testing.T) {
	current := mustMapper(t, chainSchema).schema
	next := mustMapper(t, strings.Replace(chainSchema, `"store": "chains"`, `"store": "outros"`, 1)).schema
	if _, err := UIDChanges(current, next, chainRecords()); err == nil || !strings.Contains(err.Error(), "same store") {
		t.Fatalf("expected an error for schemas selecting other items, got %v", err)
	}
}
