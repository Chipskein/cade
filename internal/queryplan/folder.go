package queryplan

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chipskein/cade/internal/config"
)

// folderInQuestion finds a path typed in the question ("na pasta
// ~/Documents"). A fixed rule, not a planner field: the model has no slot
// for paths and folded "~/Documents" into the topic, so files from every
// folder answered.
var folderInQuestion = regexp.MustCompile(`(?:^|[\s"'(])(~/[^\s"'?,;)]*|/[^\s"'?,;)]+)`)

// folderTrailing is punctuation that ends a sentence, not the path.
const folderTrailing = ".:!"

// FolderIn returns the absolute folder the question names, with "~"
// expanded against home, or "" when it names none.
//
//	queryplan.FolderIn("imagens em ~/Documents?", "/home/ana") // "/home/ana/Documents"
func FolderIn(question, home string) string {
	match := folderInQuestion.FindStringSubmatch(question)
	if match == nil {
		return ""
	}
	path := strings.TrimRight(match[1], folderTrailing)
	return filepath.Clean(config.ExpandHomeIn(path, home))
}
