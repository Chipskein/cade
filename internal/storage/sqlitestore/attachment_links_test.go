package sqlitestore

import (
	"context"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/testcheck"
)

func describedImage(uid, path, sha, description string) event.Event {
	metadata := event.File{Path: path}.Metadata()
	for key, value := range (event.Image{SHA256: sha, Status: event.CaptionDescribed, Description: description}).Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: uid, Source: event.SourceFile, Timestamp: baseTime, Content: path + "\nImagem: " + description, Metadata: metadata}
}

func photoMessage(uid, attachment string) event.Event {
	message := event.Message{Sender: "Pedro Alves", Conversation: "Pedro Alves", Kind: event.KindOther}
	metadata := message.Metadata()
	metadata[event.AttachmentKey] = attachment
	return event.Event{UID: uid, Source: event.SourceWhatsApp, Timestamp: baseTime, Content: message.Content(), Metadata: metadata}
}

// shownImages are the keys of the images uid shows.
func shownImages(t *testing.T, store *Store, uid string) []string {
	t.Helper()
	links, err := store.EntitiesOfEvent(context.Background(), uid)
	testcheck.NoError(t, err)
	var keys []string
	for _, link := range links {
		if link.Type == entity.TypeImage && link.Role == entity.RoleShows {
			keys = append(keys, link.Key)
		}
	}
	return keys
}

func TestAMessageShowsTheImageItAttachesWhicheverIsStoredFirst(t *testing.T) {
	file := describedImage("file", "/WhatsApp Images/IMG-1.jpg", "sha-1", "comprovante para João")
	orders := map[string][]event.Event{
		"file first":    {file, photoMessage("photo", "IMG-1.jpg")},
		"message first": {photoMessage("photo", "IMG-1.jpg"), file},
	}
	for name, events := range orders {
		store := openTestStore(t)
		for _, ev := range events {
			mustSave(t, store, ev, []float32{1, 0})
		}
		mustSave(t, store, photoMessage("other", "IMG-2.jpg"), []float32{1, 0})
		if got := shownImages(t, store, "photo"); !slices.Equal(got, []string{"sha-1"}) {
			t.Errorf("%s: expected the photo to show sha-1, got %v", name, got)
		}
		if got := shownImages(t, store, "other"); len(got) != 0 {
			t.Errorf("%s: expected a message naming another file to show nothing, got %v", name, got)
		}
	}
}

// The description belongs to the Image (#22): it stays while a message
// still shows the picture.
func TestForgettingTheFileKeepsTheImageAMessageShows(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, describedImage("file", "/w/IMG-1.jpg", "sha-1", "comprovante"), []float32{1, 0})
	mustSave(t, store, photoMessage("photo", "IMG-1.jpg"), []float32{1, 0})
	_, err := store.DeleteEvent(context.Background(), "file")
	testcheck.NoError(t, err)
	if got := shownImages(t, store, "photo"); !slices.Equal(got, []string{"sha-1"}) {
		t.Fatalf("expected the image kept for the message, got %v", got)
	}
}

func TestAttachmentMigrationLinksStoredMessages(t *testing.T) {
	legacy := newLegacyDatabase(t, 4)
	legacy.add(photoMessage("photo", "IMG-1.jpg"), []float32{1, 0})
	legacy.add(describedImage("file", "/w/IMG-1.jpg", "sha-1", "comprovante"), []float32{0, 1})
	legacy.close()
	reopened, err := Open(context.Background(), legacy.Path)
	testcheck.NoError(t, err)
	defer reopened.Close()
	if got := shownImages(t, reopened, "photo"); !slices.Equal(got, []string{"sha-1"}) || versionOf(t, reopened.db) != latestVersion() {
		t.Fatalf("expected the stored photo linked after the migration, got %v", got)
	}
}
