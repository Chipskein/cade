package discovery

import (
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/testfakes"
)

const home = "/home/ana"

func fakeProfiles() testfakes.FakeFileSystem {
	return testfakes.NewFakeFileSystem().
		AddFile(home+"/.config/google-chrome/Default/History", "").
		AddFile(home+"/.config/google-chrome/Profile 1/History", "").
		AddDir(home+"/.config/google-chrome/Default/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb").
		AddDir(home+"/.config/google-chrome/Default/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb").
		AddDir(home+"/.config/google-chrome/Default/IndexedDB/https_www.example.com_0.indexeddb.leveldb").
		AddDir(home+"/.config/google-chrome/Crashpad").
		AddFile(home+"/.mozilla/firefox/abc.default-release/places.sqlite", "").
		AddFile(home+"/.mozilla/firefox/profiles.ini", "").
		AddFile(home+"/.floorp/xyz.default-default/places.sqlite", "").
		AddDir(home + "/.floorp/xyz.default-default/storage/default/https+++teams.microsoft.com/idb").
		AddDir(home + "/.floorp/xyz.default-default/storage/default/https+++teams.cloud.microsoft/cache").
		AddDir(home + "/.floorp/xyz.default-default/storage/default/https+++web.whatsapp.com/idb")
}

func TestBrowserHistoriesFindsEveryProfile(t *testing.T) {
	want := []string{
		home + "/.config/google-chrome/Default/History",
		home + "/.config/google-chrome/Profile 1/History",
		home + "/.floorp/xyz.default-default/places.sqlite",
		home + "/.mozilla/firefox/abc.default-release/places.sqlite",
	}
	if got := BrowserHistories(fakeProfiles(), home, RootsFor("linux")); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBrowserHistoriesEmptyHome(t *testing.T) {
	if got := BrowserHistories(testfakes.NewFakeFileSystem(), home, RootsFor("linux")); len(got) != 0 {
		t.Fatalf("expected none, got %v", got)
	}
}

func TestTeamsCachesKeepsOnlyTeamsOrigins(t *testing.T) {
	indexedDB := home + "/.config/google-chrome/Default/IndexedDB/"
	want := []string{
		indexedDB + "https_teams.cloud.microsoft_0.indexeddb.leveldb",
		indexedDB + "https_teams.microsoft.com_0.indexeddb.leveldb",
		home + "/.floorp/xyz.default-default/storage/default/https+++teams.microsoft.com/idb",
	}
	if got := TeamsCaches(fakeProfiles(), home, RootsFor("linux")); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// FreeBSD has the ~/.config and ~/.mozilla layouts, but no snap or
// flatpak (#101).
func TestRootsForFreeBSDLeaveOutSnapAndFlatpak(t *testing.T) {
	tree := testfakes.NewFakeFileSystem().
		AddFile(home+"/.config/chromium/Default/History", "").
		AddFile(home+"/snap/firefox/common/.mozilla/firefox/x.default/places.sqlite", "").
		AddFile(home+"/.mozilla/firefox/y.default/places.sqlite", "")
	want := []string{home + "/.config/chromium/Default/History", home + "/.mozilla/firefox/y.default/places.sqlite"}
	if got := BrowserHistories(tree, home, RootsFor("freebsd")); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got := BrowserHistories(tree, home, RootsFor("linux")); len(got) != 3 {
		t.Fatalf("expected Linux to see the snap profile too, got %v", got)
	}
}

// macOS keeps the profiles under Application Support (#53).
func TestRootsForMacOSAreUnderApplicationSupport(t *testing.T) {
	support := home + "/Library/Application Support/"
	tree := testfakes.NewFakeFileSystem().
		AddFile(support+"Google/Chrome/Default/History", "").
		AddDir(support+"Google/Chrome/Default/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb").
		AddFile(support+"Firefox/Profiles/abc.default-release/places.sqlite", "").
		AddFile(home+"/.mozilla/firefox/y.default/places.sqlite", "")
	want := []string{support + "Firefox/Profiles/abc.default-release/places.sqlite", support + "Google/Chrome/Default/History"}
	if got := BrowserHistories(tree, home, RootsFor("darwin")); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got := TeamsCaches(tree, home, RootsFor("darwin")); len(got) != 1 {
		t.Fatalf("expected the Teams cache of Chrome, got %v", got)
	}
}

func TestRootsForAnUnknownSystemAreLinuxs(t *testing.T) {
	if !slices.Equal(RootsFor("netbsd").Firefox, RootsFor("linux").Firefox) {
		t.Fatal("expected Linux's roots for a system without its own")
	}
}

func fakeSourceTree() testfakes.FakeFileSystem {
	return testfakes.NewFakeFileSystem().
		AddDir("/src/api/.git").
		AddFile("/src/api/nested/.git", "gitdir: elsewhere").
		AddFile("/src/worktree/.git", "gitdir: /src/api/.git/worktrees/w").
		AddDir("/src/org/team/web/.git").
		AddDir("/src/node_modules/lib/.git").
		AddDir("/src/.cache/tool/.git").
		AddDir("/src/a/b/c/d/e/deep/.git").
		AddFile("/src/notes.md", "")
}

func TestGitRepositoriesFindsRepositoriesOnce(t *testing.T) {
	want := []string{"/src/api", "/src/org/team/web", "/src/worktree"}
	if got := GitRepositories(fakeSourceTree(), "/src", []string{"node_modules"}); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestGitRepositoriesRootItselfIsRepository(t *testing.T) {
	if got := GitRepositories(fakeSourceTree(), "/src/api", nil); !slices.Equal(got, []string{"/src/api"}) {
		t.Fatalf("expected the root, got %v", got)
	}
}

func TestGitRepositoriesMissingRoot(t *testing.T) {
	if got := GitRepositories(fakeSourceTree(), "/absent", nil); len(got) != 0 {
		t.Fatalf("expected none, got %v", got)
	}
}

func TestDepthBelow(t *testing.T) {
	cases := []struct {
		root, name string
		want       int
	}{{"src", "src", 0}, {"src", "src/a", 1}, {"src", "src/a/b", 2}, {".", "home", 1}, {".", "home/ana", 2}}
	for _, c := range cases {
		if got := depthBelow(c.root, c.name); got != c.want {
			t.Errorf("depthBelow(%q, %q) = %d, want %d", c.root, c.name, got, c.want)
		}
	}
}
