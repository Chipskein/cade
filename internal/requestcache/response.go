package requestcache

import (
	"fmt"
	"net/url"

	"github.com/chipskein/cade/internal/httpbody"
	"github.com/chipskein/cade/internal/jsonvalue"
	"github.com/chipskein/cade/internal/webstore"
)

// Response is one cached response whose URL is in the scope, as a reader
// found it.
type Response struct {
	URL string
	// Namespace is the cache a Cache API response was put in; the HTTP
	// cache has one cache per profile, so it leaves it empty.
	Namespace string
	// ContentEncoding is the response's header, "" when the body is kept
	// decoded (the Cache API keeps it so).
	ContentEncoding string
	Body            []byte
}

// Record turns r into the record of container, the scope name its URL
// matched. A body that does not decode to JSON is still a record, with
// DecodeErr set, so schema checks count it instead of losing it.
//
//	record := response.Record(webstore.KindHTTPCache, "discord-messages")
func (r Response) Record(kind webstore.Kind, container string) webstore.Record {
	record := webstore.Record{Kind: kind, Origin: originOf(r.URL), Namespace: r.Namespace, Container: container, Key: r.URL}
	plain, err := httpbody.Decode(r.ContentEncoding, r.Body)
	if err == nil {
		record.Value, err = jsonvalue.Parse(plain)
	}
	if err != nil {
		record.DecodeErr = fmt.Errorf("body of %s: %w", r.URL, err)
	}
	return record
}

// originOf is the scheme and host of rawURL, as a record's origin; the
// scope only lets in URLs with both.
func originOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
