package filesource

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chipskein/cade/internal/event"
)

var modified = time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC)

func sampleTree() fstest.MapFS {
	return fstest.MapFS{
		"notes/todo.md":       {Data: []byte("# TODO\nrevisar PR"), ModTime: modified},
		"image.png":           {Data: []byte{0x89, 'P', 'N', 'G', 0}, ModTime: modified},
		"big.log":             {Data: []byte(strings.Repeat("x", 100)), ModTime: modified},
		".git/HEAD":           {Data: []byte("ref"), ModTime: modified},
		"node_modules/a/b.js": {Data: []byte("js"), ModTime: modified},
	}
}

func collectTree(t *testing.T, tree fstest.MapFS) map[string]event.Event {
	t.Helper()
	opts := Options{IgnoredDirNames: []string{".git", "node_modules"}, MaxFileBytes: 50}
	byPath := map[string]event.Event{}
	err := NewCollector(tree, "/root", opts).CollectEvents(context.Background(), func(ev event.Event) error {
		byPath[ev.Metadata["path"]] = ev
		return nil
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return byPath
}

func TestCollectSkipsIgnoredDirectories(t *testing.T) {
	events := collectTree(t, sampleTree())
	if len(events) != 3 {
		t.Fatalf("expected 3 files outside ignored dirs, got %v", keys(events))
	}
}

func TestCollectReadsTextContent(t *testing.T) {
	ev := collectTree(t, sampleTree())["/root/notes/todo.md"]
	if ev.Content != "todo.md\n# TODO\nrevisar PR" || !ev.Timestamp.Equal(modified) || ev.Source != event.SourceFile {
		t.Fatalf("unexpected event %+v", ev)
	}
}

func TestCollectKeepsBinaryAndLargeFilesWithoutContent(t *testing.T) {
	events := collectTree(t, sampleTree())
	if events["/root/image.png"].Content != "image.png" || events["/root/big.log"].Content != "big.log" {
		t.Fatalf("expected path-only content, got %q and %q", events["/root/image.png"].Content, events["/root/big.log"].Content)
	}
}

func TestEditedFileGetsNewID(t *testing.T) {
	before := collectTree(t, sampleTree())["/root/notes/todo.md"]
	tree := sampleTree()
	tree["notes/todo.md"].ModTime = modified.Add(time.Minute)
	after := collectTree(t, tree)["/root/notes/todo.md"]
	if before.UID == after.UID {
		t.Fatal("an edit must produce a new event")
	}
}

func TestCollectStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewCollector(sampleTree(), "/root", Options{}).CollectEvents(ctx, func(event.Event) error { return nil })
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestLooksLikeText(t *testing.T) {
	if !looksLikeText([]byte("olá")) || looksLikeText([]byte{'a', 0}) || looksLikeText([]byte{0xff, 0xfe}) {
		t.Fatal("expected UTF-8 without NUL to be text, and NUL or invalid UTF-8 not")
	}
}

func keys(events map[string]event.Event) []string {
	var names []string
	for name := range events {
		names = append(names, name)
	}
	return names
}
