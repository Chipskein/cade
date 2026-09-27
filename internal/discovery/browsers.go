// Package discovery finds what `cade init` can offer to ingest: browser
// histories, Teams web caches and git repositories. It looks at names
// only and never reads a file's content.
package discovery

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/chipskein/cade/internal/rootfs"
)

// chromiumRoots are the user-data directories of Chromium-based browsers
// on Linux, relative to home; each holds one directory per profile.
var chromiumRoots = []string{
	".config/google-chrome", ".config/google-chrome-beta", ".config/chromium",
	".config/BraveSoftware/Brave-Browser", ".config/microsoft-edge", ".config/vivaldi",
	"snap/chromium/common/chromium",
}

// firefoxRoots hold Firefox profiles: the usual install, snap and flatpak.
var firefoxRoots = []string{
	".mozilla/firefox", "snap/firefox/common/.mozilla/firefox", ".var/app/org.mozilla.firefox/.mozilla/firefox",
}

// Teams on the web keeps its cache in one IndexedDB per origin
// (teams.microsoft.com, teams.cloud.microsoft).
const (
	teamsIndexedDBPrefix = "https_teams."
	teamsIndexedDBSuffix = ".indexeddb.leveldb"
)

// BrowserHistories returns the Chromium History and Firefox places.sqlite
// files of every profile under home, sorted.
//
//	histories := discovery.BrowserHistories(os.DirFS("/"), "/home/me")
func BrowserHistories(fsys fs.FS, home string) []string {
	histories := filesInProfiles(fsys, home, chromiumRoots, "History")
	histories = append(histories, filesInProfiles(fsys, home, firefoxRoots, "places.sqlite")...)
	sort.Strings(histories)
	return histories
}

// TeamsCaches returns the Teams IndexedDB directories of every Chromium
// profile under home, sorted.
func TeamsCaches(fsys fs.FS, home string) []string {
	var caches []string
	for _, indexedDB := range filesInProfiles(fsys, home, chromiumRoots, "IndexedDB") {
		caches = append(caches, teamsDirsIn(fsys, indexedDB)...)
	}
	sort.Strings(caches)
	return caches
}

func teamsDirsIn(fsys fs.FS, indexedDB string) []string {
	entries, err := fs.ReadDir(fsys, rootfs.Name(indexedDB))
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && strings.HasPrefix(name, teamsIndexedDBPrefix) && strings.HasSuffix(name, teamsIndexedDBSuffix) {
			dirs = append(dirs, path.Join(indexedDB, name))
		}
	}
	return dirs
}

// filesInProfiles returns marker's path in every profile directory of the
// given roots that has it.
func filesInProfiles(fsys fs.FS, home string, roots []string, marker string) []string {
	var found []string
	for _, root := range roots {
		for _, profile := range profileDirs(fsys, path.Join(home, root)) {
			candidate := path.Join(profile, marker)
			if _, err := fs.Stat(fsys, rootfs.Name(candidate)); err == nil {
				found = append(found, candidate)
			}
		}
	}
	return found
}

func profileDirs(fsys fs.FS, root string) []string {
	entries, err := fs.ReadDir(fsys, rootfs.Name(root))
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, path.Join(root, entry.Name()))
		}
	}
	return dirs
}
