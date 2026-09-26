package event

import (
	"reflect"
	"testing"
	"time"
)

func TestTypedMetadataRoundTrips(t *testing.T) {
	commit := Commit{Repository: "/src/api", Hash: "79589eae", Author: "Eu", Email: "eu@example.com", Files: []string{"a.go", "b.go"}}
	visit := Visit{Browser: "chrome", URL: "https://go.dev", Title: "Go", History: "/h/History"}
	file := File{Path: "/notas/a.md", Size: 1234}
	message := Message{ConversationID: "19:x", Conversation: "Ana, Eu", Kind: KindChat, MessageID: "1", Sender: "Ana",
		SenderMRI: "8:orgid:ana", SentByMe: true, Origin: "https_teams.microsoft.com_0", Revision: "17"}
	if got := (Event{Metadata: commit.Metadata()}).Commit(); !reflect.DeepEqual(got, commit) {
		t.Errorf("commit: expected %+v, got %+v", commit, got)
	}
	if got := (Event{Metadata: visit.Metadata()}).Visit(); got != visit {
		t.Errorf("visit: expected %+v, got %+v", visit, got)
	}
	if got := (Event{Metadata: file.Metadata()}).File(); got != file {
		t.Errorf("file: expected %+v, got %+v", file, got)
	}
	if got := (Event{Metadata: message.Metadata()}).Message(); got != message {
		t.Errorf("message: expected %+v, got %+v", message, got)
	}
}

// Stored events keep the keys written before the typed views existed.
func TestTypedMetadataReadsStoredKeys(t *testing.T) {
	stored := Event{Metadata: Metadata{"sender": "Ana", "sent_by_me": "false", "conversation_kind": "canal", "message_id": "9"}}
	message := stored.Message()
	if message.Sender != "Ana" || message.SentByMe || message.Kind != KindChannel || message.MessageID != "9" {
		t.Fatalf("unexpected message %+v", message)
	}
	if (Event{Metadata: Metadata{"files": ""}}).Commit().Files != nil {
		t.Fatal("expected no files for an empty list")
	}
}

func TestMissingMetadataReadsAsZero(t *testing.T) {
	var empty Event
	if empty.Commit().Hash != "" || empty.File().Size != 0 || empty.Message().SentByMe {
		t.Fatal("expected zero values")
	}
}

func TestFileTimesRoundTripAsRevision(t *testing.T) {
	modified, removed := time.Unix(1758800000, 123456789), time.Unix(1758900000, 0)
	file := File{Path: "/notas/a.md", Size: 10, ModifiedAt: modified, RemovedAt: removed}
	ev := Event{Metadata: file.Metadata()}
	revision, ok := ev.Revision()
	if got := ev.File(); !got.ModifiedAt.Equal(modified) || !got.RemovedAt.Equal(removed) || !ok || revision != modified.UnixNano() {
		t.Fatalf("expected the times back and the modification as revision, got %+v (revision %d)", got, revision)
	}
	if !(Event{Metadata: File{Path: "/a"}.Metadata()}).File().RemovedAt.IsZero() {
		t.Fatal("expected a present file to have no removal time")
	}
}
