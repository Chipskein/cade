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
		AddFile(home+"/.mozilla/firefox/profiles.ini", "")
}

func TestBrowserHistoriesFindsEveryProfile(t *testing.T) {
	want := []string{
		home + "/.config/google-chrome/Default/History",
		home + "/.config/google-chrome/Profile 1/History",
		home + "/.mozilla/firefox/abc.default-release/places.sqlite",
	}
	if got := BrowserHistories(fakeProfiles(), home); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestBrowserHistoriesEmptyHome(t *testing.T) {
	if got := BrowserHistories(testfakes.NewFakeFileSystem(), home); len(got) != 0 {
		t.Fatalf("expected none, got %v", got)
	}
}

func TestTeamsCachesKeepsOnlyTeamsOrigins(t *testing.T) {
	indexedDB := home + "/.config/google-chrome/Default/IndexedDB/"
	want := []string{
		indexedDB + "https_teams.cloud.microsoft_0.indexeddb.leveldb",
		indexedDB + "https_teams.microsoft.com_0.indexeddb.leveldb",
	}
	if got := TeamsCaches(fakeProfiles(), home); !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
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
