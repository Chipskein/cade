package discovery

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/rootfs"
)

// maxRepositoryDepth bounds the walk below the directory the user names:
// ~/src/org/project is depth 2, and a whole home directory would otherwise
// be walked file by file.
const maxRepositoryDepth = 4

// GitRepositories returns the directories under root (root included) that
// hold a .git entry, sorted. It does not descend into a repository, into
// hidden directories or into skipped names (node_modules, vendor...), and
// unreadable directories are passed over.
//
//	repos := discovery.GitRepositories(os.DirFS("/"), "/home/me/src", []string{"node_modules"})
func GitRepositories(fsys fs.FS, root string, skipped []string) []string {
	rootName := rootfs.Name(root)
	var repositories []string
	visit := func(name string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		if (name != rootName && skipDirectory(entry.Name(), skipped)) || depthBelow(rootName, name) > maxRepositoryDepth {
			return fs.SkipDir
		}
		if _, statErr := fs.Stat(fsys, path.Join(name, ".git")); statErr == nil {
			repositories = append(repositories, rootfs.Path(name))
			return fs.SkipDir
		}
		return nil
	}
	_ = fs.WalkDir(fsys, rootName, visit)
	slices.Sort(repositories)
	return repositories
}

func skipDirectory(name string, skipped []string) bool {
	return strings.HasPrefix(name, ".") || slices.Contains(skipped, name)
}

func depthBelow(rootName, name string) int {
	if name == rootName {
		return 0
	}
	relative := name
	if rootName != "." {
		relative = strings.TrimPrefix(name, rootName+"/")
	}
	return strings.Count(relative, "/") + 1
}
