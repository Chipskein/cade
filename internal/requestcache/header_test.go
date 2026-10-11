package requestcache

import "testing"

func TestHeaderValueFindsTheHeaderInAnyCase(t *testing.T) {
	firefox := []string{"HTTP/2 200 ", "Content-Type: application/json", "Content-Encoding: br"}
	chromium := []string{"HTTP/1.1 200", "content-type:application/json", "content-encoding:gzip"}
	if got := HeaderValue(firefox, HeaderContentEncoding); got != "br" {
		t.Errorf("HeaderValue(firefox) = %q; want br", got)
	}
	if got := HeaderValue(chromium, HeaderContentEncoding); got != "gzip" {
		t.Errorf("HeaderValue(chromium) = %q; want gzip", got)
	}
}

func TestHeaderValueIsEmptyWhenAbsent(t *testing.T) {
	if got := HeaderValue([]string{"HTTP/1.1 200", "content-type: text/html"}, HeaderContentEncoding); got != "" {
		t.Fatalf("HeaderValue = %q; want empty", got)
	}
}
