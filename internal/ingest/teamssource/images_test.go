package teamssource

import (
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/event"
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
