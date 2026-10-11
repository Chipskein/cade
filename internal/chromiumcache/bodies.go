package chromiumcache

import (
	"github.com/chipskein/cade/internal/requestcache"
	"github.com/chipskein/cade/internal/simplecache"
	"github.com/chipskein/cade/internal/webstore"
)

// ResponseBodies reads from the HTTP cache dir the raw bodies of the
// responses whose URL wanted keeps, by URL: the pictures shown in Teams
// messages, which are cached as the browser displays them (#24). A body
// sent compressed is left out; pictures are not sent that way. The body
// of a URL not wanted is never read.
//
//	bodies, err := chromiumcache.ResponseBodies("~/.cache/google-chrome/Default/Cache/Cache_Data", wanted)
func ResponseBodies(dir string, wanted func(url string) bool) (map[string][]byte, error) {
	bodies := map[string][]byte{}
	_, err := readEntryFiles(dir, func(entry simplecache.Entry) (webstore.Record, bool) {
		url := keyPrefix.ReplaceAllString(entry.Key, "")
		if !wanted(url) {
			return webstore.Record{}, false
		}
		stream0, body, err := entry.Streams()
		if err == nil && requestcache.HeaderValue(rawHeaderLines(stream0), requestcache.HeaderContentEncoding) == "" {
			bodies[url] = body
		}
		// Nothing is collected as a record: the bodies are the result.
		return webstore.Record{}, false
	})
	return bodies, err
}
