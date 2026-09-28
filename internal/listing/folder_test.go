package listing

import (
	"testing"

	"github.com/chipskein/cade/internal/event"
)

func folderTestFile(uid, path string) event.Event {
	return event.Event{UID: uid, Source: event.SourceFile, Metadata: event.File{Path: path}.Metadata()}
}

func TestInFolderKeepsOnlyFilesUnderIt(t *testing.T) {
	all := []event.Event{folderTestFile("in", "/home/ana/Documents/a.png"), folderTestFile("sibling", "/home/ana/Documents2/b.png"),
		folderTestFile("out", "/home/ana/Downloads/c.png"), {UID: "commit", Source: event.SourceGit}}
	kept := InFolder(all, "/home/ana/Documents/")
	if len(kept) != 1 || kept[0].UID != "in" || len(InFolder(all, "")) != len(all) {
		t.Fatalf("expected only the file under the folder, got %+v", kept)
	}
}
