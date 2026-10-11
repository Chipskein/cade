package teamssource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/testcheck"
)

func TestHTTPCacheDirsOfALinuxChromeProfile(t *testing.T) {
	dirs := HTTPCacheDirs("/home/ana/.config/google-chrome/Default/IndexedDB/https_teams.microsoft.com_0.indexeddb.leveldb",
		"/home/ana/.config", "/home/ana/.cache")
	want := []string{"/home/ana/.cache/google-chrome/Default/Cache/Cache_Data", "/home/ana/.config/google-chrome/Default/Cache/Cache_Data"}
	if !slices.Equal(dirs, want) {
		t.Fatalf("expected the XDG cache first, then inside the profile, got %v", dirs)
	}
}

func TestHTTPCacheDirsOfAProfileOutsideTheConfigDir(t *testing.T) {
	dirs := HTTPCacheDirs(`/Users/ana/Library/Application Support/Google/Chrome/Default/IndexedDB/x.leveldb`, "/home/ana/.config", "/home/ana/.cache")
	if !slices.Equal(dirs, []string{"/Users/ana/Library/Application Support/Google/Chrome/Default/Cache/Cache_Data"}) {
		t.Fatalf("expected only the cache inside the profile, got %v", dirs)
	}
}

// Without a cache directory there is nothing to read, and no error: the
// messages keep only the addresses of their pictures.
func TestPicturesWithoutACacheAreNone(t *testing.T) {
	reader := FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(message("1", "RichText/Html", amsTag(pastedImage)))}}
	collector := NewCollector(reader.Read, "/p/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb")
	pictures, err := collector.Pictures(context.Background(), []string{filepath.Join(t.TempDir(), "absent")})
	if err != nil || len(pictures) != 0 {
		t.Fatalf("expected no pictures and no error, got %d (err %v)", len(pictures), err)
	}
}

func TestPicturesOfAMessageWithoutImagesSkipTheCache(t *testing.T) {
	reader := FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(message("1", "Text", "oi"))}}
	cache := t.TempDir()
	testcheck.NoError(t, os.Chmod(cache, 0o000)) // reading it would fail
	t.Cleanup(func() { testcheck.NoError(t, os.Chmod(cache, 0o700)) })
	pictures, err := NewCollector(reader.Read, "/p/IndexedDB/x.leveldb").Pictures(context.Background(), []string{cache})
	if err != nil || pictures != nil {
		t.Fatalf("expected the cache left unread, got %v (err %v)", pictures, err)
	}
}
