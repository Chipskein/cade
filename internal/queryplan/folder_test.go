package queryplan

import "testing"

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
