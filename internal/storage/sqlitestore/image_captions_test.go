package sqlitestore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chipskein/cade/internal/event"
)

// imageFileEvent is a file event at path whose image has hash sha256.
func imageFileEvent(path, sha256 string, status event.CaptionStatus, visibleText string) event.Event {
	image := event.Image{SHA256: sha256, Width: 800, Height: 600, Status: status, Model: "Qwen3.5-2B-Q4_K_M.gguf", PromptVersion: 1,
		Description: "um terminal", VisibleText: visibleText}
	metadata := event.File{Path: path, Size: 10, ModifiedAt: baseTime}.Metadata()
	for key, value := range image.Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile, Timestamp: baseTime,
		Content: filepath.Base(path) + "\n" + image.SearchableText(), Metadata: metadata}
}

func TestDescribedImageFindsADescribedCopyOnly(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	mustSave(t, store, imageFileEvent("/prints/a.png", "pending-hash", event.CaptionPending, ""), nil)
	mustSave(t, store, imageFileEvent("/prints/b.png", "described-hash", event.CaptionDescribed, "panic: nil map"), nil)
	image, found, err := store.DescribedImage(ctx, "described-hash")
	if err != nil || !found || image.VisibleText != "panic: nil map" || image.Width != 800 {
		t.Fatalf("expected the described image, got %+v %v (err %v)", image, found, err)
	}
	if _, found, _ := store.DescribedImage(ctx, "pending-hash"); found {
		t.Fatal("expected a pending image not to count as described")
	}
}

// Acceptance (phase 19): `forget file` erases the descriptions, from the
// tables and from the file on disk.
func TestForgetFileLeavesNoImageDescription(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	mustSave(t, store, imageFileEvent("/prints/erro.png", "hash-1", event.CaptionDescribed, "senha-do-print-5931"), []float32{1, 0})
	if _, err := store.DeleteSource(context.Background(), event.SourceFile); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.DescribedImage(context.Background(), "hash-1"); found {
		t.Fatal("expected no description left to reuse after forget")
	}
	store.Close()
	for _, suffix := range []string{"", "-wal"} {
		raw, _ := os.ReadFile(path + suffix)
		if bytes.Contains(raw, []byte("senha-do-print-5931")) {
			t.Fatalf("forgotten image text still in %s", path+suffix)
		}
	}
}
