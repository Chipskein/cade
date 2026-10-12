// Package discovery finds what `cade init` can offer to ingest: browser
// histories, Teams web caches and git repositories. It looks at names
// only and never reads a file's content.
package discovery

import (
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/chipskein/cade/internal/rootfs"
)

// BrowserRoots are where one system's browsers keep their profiles,
// relative to home: Chromium-based user-data directories, each with one
// directory per profile, and Firefox (or Floorp, a fork with the same
// layout) profile directories. TeamsApps are the user-data directories of
// desktop apps built on a Chromium WebView, read only for their Teams
// cache: their history is the app's, not the user's browsing.
type BrowserRoots struct {
	Chromium  []string
	Firefox   []string
	TeamsApps []string
}

// xdgChromiumRoots and xdgFirefoxRoots follow ~/.config and ~/.mozilla, as
// on Linux and FreeBSD (the ports' Chromium and the Linux browsers that
// linux-browser-installer runs there).
var (
	xdgChromiumRoots = []string{
		".config/google-chrome", ".config/google-chrome-beta", ".config/chromium",
		".config/BraveSoftware/Brave-Browser", ".config/microsoft-edge", ".config/vivaldi",
	}
	xdgFirefoxRoots = []string{".mozilla/firefox", ".floorp"}
)

// macSupport is where macOS apps keep their data, relative to home.
const macSupport = "Library/Application Support/"

// Windows keeps Chromium profiles under %LOCALAPPDATA% and Firefox's
// under %APPDATA%, relative to home in their default places.
const (
	windowsLocal   = "AppData/Local/"
	windowsRoaming = "AppData/Roaming/"
)

// windowsTeamsWebView is the new Teams desktop app's WebView2 user data;
// its WV2Profile_tfw profile keeps the same IndexedDB as Teams on the web
// (#54). Not checked on a real machine yet.
const windowsTeamsWebView = windowsLocal + "Packages/MSTeams_8wekyb3d8bbwe/LocalCache/Microsoft/MSTeams/EBWebView"

// browserRoots are the roots per system (runtime.GOOS); snap and flatpak
// exist only on Linux. On macOS each browser keeps its profiles under
// Application Support (#53).
var browserRoots = map[string]BrowserRoots{
	"linux": {
		Chromium: append(slices.Clone(xdgChromiumRoots), "snap/chromium/common/chromium"),
		Firefox: append(slices.Clone(xdgFirefoxRoots), "snap/firefox/common/.mozilla/firefox",
			".var/app/org.mozilla.firefox/.mozilla/firefox", ".var/app/one.ablaze.floorp/.floorp"),
	},
	"freebsd": {Chromium: xdgChromiumRoots, Firefox: xdgFirefoxRoots},
	"darwin": {
		Chromium: []string{
			macSupport + "Google/Chrome", macSupport + "Google/Chrome Beta", macSupport + "Chromium",
			macSupport + "BraveSoftware/Brave-Browser", macSupport + "Microsoft Edge", macSupport + "Vivaldi",
		},
		Firefox: []string{macSupport + "Firefox/Profiles", macSupport + "Floorp/Profiles"},
	},
	"windows": {
		Chromium: []string{
			windowsLocal + "Google/Chrome/User Data", windowsLocal + "Google/Chrome Beta/User Data", windowsLocal + "Chromium/User Data",
			windowsLocal + "BraveSoftware/Brave-Browser/User Data", windowsLocal + "Microsoft/Edge/User Data", windowsLocal + "Vivaldi/User Data",
		},
		Firefox:   []string{windowsRoaming + "Mozilla/Firefox/Profiles", windowsRoaming + "Floorp/Profiles"},
		TeamsApps: []string{windowsTeamsWebView},
	},
}

// RootsFor are the browser roots of goos; a system without its own gets
// Linux's, the most complete of the ~/.config layouts.
//
//	roots := discovery.RootsFor(runtime.GOOS)
func RootsFor(goos string) BrowserRoots {
	if roots, known := browserRoots[goos]; known {
		return roots
	}
	return browserRoots["linux"]
}

// Teams on the web keeps its cache in one IndexedDB per origin
// (teams.microsoft.com, teams.cloud.microsoft).
// Firefox keeps it under storage/default/<origin>/idb, the origin
// spelled with "+++" for "://".
const (
	teamsIndexedDBPrefix      = "https_teams."
	teamsIndexedDBSuffix      = ".indexeddb.leveldb"
	firefoxOriginStorage      = "storage/default"
	firefoxTeamsOriginPrefix  = "https+++teams."
	firefoxIndexedDBDirectory = "idb"
)

// BrowserHistories returns the Chromium History and Firefox places.sqlite
// files of every profile under home, sorted.
//
//	histories := discovery.BrowserHistories(os.DirFS("/"), "/home/me")
func BrowserHistories(fsys fs.FS, home string, roots BrowserRoots) []string {
	histories := filesInProfiles(fsys, home, roots.Chromium, "History")
	histories = append(histories, filesInProfiles(fsys, home, roots.Firefox, "places.sqlite")...)
	sort.Strings(histories)
	return histories
}

// TeamsCaches returns the Teams IndexedDB directories of every Chromium
// and Firefox profile under home, sorted.
func TeamsCaches(fsys fs.FS, home string, roots BrowserRoots) []string {
	var caches []string
	chromiumRoots := append(slices.Clone(roots.Chromium), roots.TeamsApps...)
	for _, indexedDB := range filesInProfiles(fsys, home, chromiumRoots, "IndexedDB") {
		caches = append(caches, teamsDirsIn(fsys, indexedDB)...)
	}
	for _, storage := range filesInProfiles(fsys, home, roots.Firefox, firefoxOriginStorage) {
		caches = append(caches, firefoxTeamsDirsIn(fsys, storage)...)
	}
	sort.Strings(caches)
	return caches
}

func teamsDirsIn(fsys fs.FS, indexedDB string) []string {
	var dirs []string
	for _, name := range subdirectories(fsys, indexedDB) {
		if strings.HasPrefix(name, teamsIndexedDBPrefix) && strings.HasSuffix(name, teamsIndexedDBSuffix) {
			dirs = append(dirs, path.Join(indexedDB, name))
		}
	}
	return dirs
}

// firefoxTeamsDirsIn returns the idb directory of each Teams origin; an
// origin without one has stored nothing in IndexedDB.
func firefoxTeamsDirsIn(fsys fs.FS, storage string) []string {
	var dirs []string
	for _, name := range subdirectories(fsys, storage) {
		idb := path.Join(storage, name, firefoxIndexedDBDirectory)
		if _, err := fs.Stat(fsys, rootfs.Name(idb)); err == nil && strings.HasPrefix(name, firefoxTeamsOriginPrefix) {
			dirs = append(dirs, idb)
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
	var dirs []string
	for _, name := range subdirectories(fsys, root) {
		dirs = append(dirs, path.Join(root, name))
	}
	return dirs
}

// subdirectories returns the names of dir's direct subdirectories; a
// missing or unreadable dir has none.
func subdirectories(fsys fs.FS, dir string) []string {
	entries, err := fs.ReadDir(fsys, rootfs.Name(dir))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}
