package sqlitestore

import (
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
)

func TestUnixMillisRoundTrip(t *testing.T) {
	moment := time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.FixedZone("BRT", -3*3600))
	if got := fromUnixMillis(toUnixMillis(moment)); !got.Equal(moment) {
		t.Fatalf("expected %s, got %s", moment, got)
	}
}

func TestEncodeNilMetadataAsEmptyObject(t *testing.T) {
	encoded, err := encodeMetadata(nil)
	if err != nil || encoded != "{}" {
		t.Fatalf("expected {}, got %q (err %v)", encoded, err)
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	encoded, _ := encodeMetadata(event.Metadata{"url": "https://x"})
	decoded, err := decodeMetadata(encoded)
	if err != nil || decoded["url"] != "https://x" {
		t.Fatalf("expected url to survive, got %v (err %v)", decoded, err)
	}
}

func TestDecodeMetadataRejectsNonObject(t *testing.T) {
	if _, err := decodeMetadata(`[1,2]`); err == nil {
		t.Fatal("expected an error for a JSON array")
	}
}
