package requestcache

import (
	"errors"
	"strings"
)

// ErrEmptyScope stops a reader before it lists a request cache: with no
// URL configured there is nothing it may read.
var ErrEmptyScope = errors.New("no URL pattern in sources.request_cache_urls, expected at least one: a request cache holds the responses of every site, so only configured URLs are read")

// HeaderContentEncoding names the header a cached body is encoded by.
const HeaderContentEncoding = "content-encoding"

// headerSeparator ends a header's name in the raw header lines both
// browsers keep ("content-encoding: br", Chromium without the space).
const headerSeparator = ":"

// HeaderValue finds the header name, in any case, among raw header lines
// (the status line among them is skipped, having no name before ":").
//
//	encoding := requestcache.HeaderValue(lines, requestcache.HeaderContentEncoding)
func HeaderValue(lines []string, name string) string {
	for _, line := range lines {
		key, value, found := strings.Cut(line, headerSeparator)
		if found && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
