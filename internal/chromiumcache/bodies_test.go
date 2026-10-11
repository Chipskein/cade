package chromiumcache

import (
	"testing"

	"github.com/chipskein/cade/internal/testfakes"
)

const (
	teamsImageURL = "https://us-api.asm.skype.com/v1/objects/0-eus-d9-aa11/views/imgo"
	jpegBody      = "\xff\xd8\xff\xe0 fake jpeg"
)

func TestResponseBodiesReadsTheWantedURLsRaw(t *testing.T) {
	dir := writeSimpleCache(t, map[string]testfakes.SimpleCacheEntry{
		"0d1c857aa3989262_0": httpEntry("1/0/_dk_https://teams.microsoft.com https://teams.microsoft.com "+teamsImageURL, "", []byte(jpegBody)),
		"1111111111111111_0": httpEntry("1/0/"+profileURL, "", []byte(`{"bio":"x"}`)),
		"2222222222222222_0": httpEntry("1/0/"+teamsImageURL+"2", "gzip", gzipped(t, jpegBody)),
	})
	wanted := func(url string) bool { return url == teamsImageURL || url == teamsImageURL+"2" }
	bodies, err := ResponseBodies(dir, wanted)
	if err != nil || len(bodies) != 1 || string(bodies[teamsImageURL]) != jpegBody {
		t.Fatalf("ResponseBodies = %q, %v; want the one uncompressed image, by URL", bodies, err)
	}
}

func TestResponseBodiesOfAMissingDirectoryFails(t *testing.T) {
	if _, err := ResponseBodies(t.TempDir()+"/absent", func(string) bool { return true }); err == nil {
		t.Fatal("expected an error for a missing cache directory")
	}
}
