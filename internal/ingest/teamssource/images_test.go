package teamssource

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/testcheck"
)

const (
	pastedImage = `https://us-api.asm.skype.com/v1/objects/0-eus-d9-aa11/views/imgo`
	otherImage  = `https://eu-api.asm.skype.com/v1/objects/0-weu-d3-bb22/views/imgo`
)

func amsTag(url string) string {
	return `<img src="` + url + `" itemtype="http://schema.skype.com/AMSImage" itemscope="png" width="250" height="140" alt="image">`
}

func TestImageURLsKeepsAMSImagesInOrderOnce(t *testing.T) {
	html := `<p>olha o erro</p>` + amsTag(pastedImage) + `<img itemtype="http://schema.skype.com/Emoji" src="https://statics.teams.cdn.office.net/smile.png">` +
		amsTag(otherImage) + amsTag(pastedImage) + amsTag("https://evil.example.com/a.png")
	if got := imageURLs(html); !slices.Equal(got, []string{pastedImage, otherImage}) {
		t.Fatalf("expected the two AMS images, no emoji, repeat or foreign address, got %v", got)
	}
}

// A picture pasted alone has no text and was dropped before #24.
func TestAMessageWithOnlyAnImageIsKept(t *testing.T) {
	value := obj("id", str("1"), "conversationId", str("c"), "messageType", str("RichText/Html"), "content", str(amsTag(pastedImage)),
		"originalArrivalTime", num(1000), "imDisplayName", str("João Mendes"))
	message, ok := parseMessage(value, nil)
	if !ok {
		t.Fatal("expected a message with only an image to be kept")
	}
	ev := message.toEvent(conversationInfo{title: "João Mendes, Eu", kind: event.KindChat}, "origin")
	if got := ev.ImageURLs(); !slices.Equal(got, []string{pastedImage}) {
		t.Fatalf("expected the image address in the event, got %v", got)
	}
}

func TestAMessageWithoutImagesHasNoImageURLs(t *testing.T) {
	value := obj("id", str("1"), "conversationId", str("c"), "messageType", str("Text"), "content", str("oi"), "originalArrivalTime", num(1000))
	message, _ := parseMessage(value, nil)
	if ev := message.toEvent(conversationInfo{kind: event.KindChat}, "origin"); ev.ImageURLs() != nil || ev.Metadata[event.ImageURLsKey] != "" {
		t.Fatalf("expected no image key, got %v", ev.Metadata)
	}
}

func describedPicture(description string) event.Image {
	return event.Image{SHA256: "sha-" + description, Width: 250, Height: 140, Status: event.CaptionDescribed, Description: description}
}

func TestCollectGivesMessagesTheirPlannedPictures(t *testing.T) {
	reader := FakeIndexedDBReader{Records: []indexeddb.Record{replyChain(
		message("1", "RichText/Html", "<p>olha</p>"+amsTag(pastedImage)+amsTag(otherImage)),
		message("2", "Text", "sem imagem"),
	)}}
	collector := NewCollector(reader.Read, "/p/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb")
	urls, err := collector.ImageURLs(context.Background())
	if err != nil || !slices.Equal(urls, []string{pastedImage, otherImage}) {
		t.Fatalf("expected both pictures to plan, got %v (err %v)", urls, err)
	}
	collector.WithCaptions(ingest.ImageCaptions{pastedImage: describedPicture("print do erro 502"), otherImage: describedPicture("diagrama da fila")})
	var events []event.Event
	testcheck.NoError(t, collector.CollectEvents(context.Background(), func(ev event.Event) error { events = append(events, ev); return nil }))
	pictured := events[slices.IndexFunc(events, func(ev event.Event) bool { return ev.ImageURLs() != nil })]
	if pictured.Image().SHA256 != "sha-print do erro 502" || !strings.Contains(pictured.Content, "Imagem: print do erro 502") ||
		!strings.Contains(pictured.Content, "Imagem: diagrama da fila") {
		t.Fatalf("expected the first picture as the event's image and both descriptions in its text, got %q %+v", pictured.Content, pictured.Image())
	}
}

func TestWithPlannedImagesLeavesUnplannedAndPendingOutOfTheText(t *testing.T) {
	ev := event.Event{Content: "Ana: olha", Metadata: event.Metadata{event.ImageURLsKey: pastedImage + "\n" + otherImage}}
	got := withPlannedImages(ev, ingest.ImageCaptions{pastedImage: {SHA256: "sha-1", Status: event.CaptionPending}})
	if got.Content != "Ana: olha" || got.Image().Status != event.CaptionPending || got.Image().SHA256 != "sha-1" {
		t.Fatalf("expected a pending picture held without text, got %q %+v", got.Content, got.Image())
	}
}
