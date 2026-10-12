package queryplan

import (
	"path/filepath"
	"testing"
)

const folderTestHome = "/home/ana"

func TestFolderInReadsTypedPaths(t *testing.T) {
	cases := map[string]string{
		"Na pasta ~/Documents liste imagens com personagens": "/home/ana/Documents",
		"imagens em ~/Downloads/fotos/?":                     "/home/ana/Downloads/fotos",
		"what is in /srv/notes.":                             "/srv/notes",
		"arquivos em \"/tmp/x\"":                             "/tmp/x",
		"quais imagens eu tenho com personagens?":            "",
		"PRs do projeto a/b":                                 "",
	}
	for question, want := range cases {
		if got := FolderIn(question, folderTestHome); got != want {
			t.Errorf("FolderIn(%q) = %q, want %q", question, got, want)
		}
	}
}

func TestFolderInReadsAWindowsPathWithSlashes(t *testing.T) {
	question := "arquivos em " + filepath.FromSlash("C:/Users/ana/notas") + "?"
	if got := FolderIn(question, folderTestHome); got != "C:/Users/ana/notas" {
		t.Fatalf("expected C:/Users/ana/notas, got %q", got)
	}
}
