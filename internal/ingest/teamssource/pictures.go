package teamssource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/chromiumcache"
)

// The browser caches each picture it shows in its HTTP cache, which a
// Chromium profile keeps apart from its IndexedDB: on Linux under
// ~/.cache/<browser>/<profile>, elsewhere inside the profile. A Firefox
// profile has another cache format, so its pictures are not read.

// httpCacheSubdir is the HTTP cache inside a Chromium cache directory.
var httpCacheSubdir = filepath.Join("Cache", "Cache_Data")

// HTTPCacheDirs are where the HTTP cache of the profile holding
// indexedDBDir may be, most likely first; configDir and cacheDir are the
// user's (os.UserConfigDir, os.UserCacheDir).
//
//	dirs := teamssource.HTTPCacheDirs("~/.config/google-chrome/Default/IndexedDB/https_teams…leveldb", "~/.config", "~/.cache")
func HTTPCacheDirs(indexedDBDir, configDir, cacheDir string) []string {
	profile := filepath.Dir(filepath.Dir(indexedDBDir))
	dirs := []string{filepath.Join(profile, httpCacheSubdir)}
	if relative, err := filepath.Rel(configDir, profile); err == nil && !strings.HasPrefix(relative, "..") {
		dirs = slices.Insert(dirs, 0, filepath.Join(cacheDir, relative, httpCacheSubdir))
	}
	return dirs
}

// Pictures are the bytes of the pictures the cached messages show that the
// first existing cache dir holds, by address; a picture the browser has
// evicted is missing, and its message keeps only the address.
func (c *Collector) Pictures(ctx context.Context, cacheDirs []string) (map[string][]byte, error) {
	urls, err := c.ImageURLs(ctx)
	if err != nil || len(urls) == 0 {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, url := range urls {
		wanted[url] = true
	}
	for _, dir := range cacheDirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return chromiumcache.ResponseBodies(dir, func(url string) bool { return wanted[url] })
		}
	}
	return nil, nil
}
