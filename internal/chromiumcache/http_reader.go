package chromiumcache

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/simplecache"
	"github.com/chipskein/cade/internal/webstore"
)

// HTTP cache keys (net/http/http_cache.cc) start with "<flags>/<upload
// id>/" and, with the cache split by site, "_dk_<site> <site> " before the
// URL, which never holds a space.
var keyPrefix = regexp.MustCompile(`^\d+/\d+/(?:_dk_\S* \S* )?`)

// Stream 0 is a pickled HttpResponseInfo whose raw headers are the status
// line and each "name:value", ended by NUL, with one more NUL after them.
const (
	statusLineStart  = "HTTP/"
	headersEnd       = "\x00\x00"
	headersSeparator = "\x00"
)

// HTTPCacheReader reads the responses of a Cache_Data directory whose URLs
// are in its scope.
type HTTPCacheReader struct {
	scope requestcache.Scope
}

// NewHTTPCacheReader builds a reader of scope, the configured URLs.
//
//	reader := chromiumcache.NewHTTPCacheReader(scope)
//	records, err := reader.Read("~/.config/discord/Cache/Cache_Data")
func NewHTTPCacheReader(scope requestcache.Scope) HTTPCacheReader {
	return HTTPCacheReader{scope: scope}
}

func (HTTPCacheReader) Kind() webstore.Kind { return webstore.KindHTTPCache }

// Recognizes a simple cache directory; a Cache API cache directory is one
// too, but a schema points at the origin directory above it.
func (HTTPCacheReader) Recognizes(location string) bool {
	return isSimpleCache(location)
}

// Read returns a record per entry in scope; see readEntryFiles for the
// entries it skips.
func (r HTTPCacheReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, requestcache.ErrEmptyScope
	}
	return readEntryFiles(location, r.readEntry)
}

func (r HTTPCacheReader) readEntry(entry simplecache.Entry) (webstore.Record, bool) {
	url := keyPrefix.ReplaceAllString(entry.Key, "")
	name, inScope := r.scope.Match(url)
	if !inScope {
		return webstore.Record{}, false
	}
	stream0, body, err := entry.Streams()
	if err != nil {
		return webstore.Record{}, false
	}
	response := requestcache.Response{URL: url, ContentEncoding: requestcache.HeaderValue(rawHeaderLines(stream0), requestcache.HeaderContentEncoding), Body: body}
	return response.Record(webstore.KindHTTPCache, name), true
}

// rawHeaderLines finds the raw headers in stream 0 by their status line,
// rather than walking the pickle's fields, which change between versions.
func rawHeaderLines(stream0 []byte) []string {
	start := bytes.Index(stream0, []byte(statusLineStart))
	if start < 0 {
		return nil
	}
	raw, _, _ := strings.Cut(string(stream0[start:]), headersEnd)
	return strings.Split(raw, headersSeparator)
}
