package teamssource

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
)

// A pasted or attached picture is an <img> in the message's HTML, marked
// as an AMS image and served from the Teams media service:
//
//	<img src="https://us-api.asm.skype.com/v1/objects/0-eus-d9-…/views/imgo"
//	     itemtype="http://schema.skype.com/AMSImage" itemscope="png" …>
//
// The browser caches it under that address, where the description is read
// from (#24). Emoji and stickers are other item types and stay out.
var (
	imageTags       = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	amsImageType    = regexp.MustCompile(`(?i)\bitemtype\s*=\s*"[^"]*/AMSImage"`)
	imageSource     = regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]+)"`)
	amsImageAddress = regexp.MustCompile(`^https://[a-z0-9.-]+/v1/objects/[A-Za-z0-9_-]+/views/[a-z_]+$`)
)

// imageURLs lists the AMS images of a message's HTML, in order, each once.
func imageURLs(html string) []string {
	var urls []string
	for _, tag := range imageTags.FindAllString(html, -1) {
		url, isImage := amsImageURL(tag)
		if isImage && !slices.Contains(urls, url) {
			urls = append(urls, url)
		}
	}
	return urls
}

// amsImageURL is the address of an AMS image tag; false for any other tag
// or an address outside the media service.
func amsImageURL(tag string) (string, bool) {
	source := imageSource.FindStringSubmatch(tag)
	if !amsImageType.MatchString(tag) || source == nil {
		return "", false
	}
	url := strings.ReplaceAll(source[1], "&amp;", "&")
	return url, amsImageAddress.MatchString(url)
}

// withPlannedImages adds to ev what was planned for its pictures: the
// first one becomes the event's image (an event holds one), and every
// description goes into its text, so the message is found by what its
// pictures show.
func withPlannedImages(ev event.Event, captions ingest.ImageCaptions) event.Event {
	var texts []string
	for _, url := range ev.ImageURLs() {
		image, planned := captions[url]
		if !planned {
			continue
		}
		if ev.Image().Status == event.CaptionNone {
			maps.Copy(ev.Metadata, image.Metadata())
		}
		texts = appendNonEmpty(texts, image.SearchableText())
	}
	if len(texts) > 0 {
		ev.Content += "\n" + strings.Join(texts, "\n")
	}
	return ev
}

func appendNonEmpty(texts []string, text string) []string {
	if text == "" {
		return texts
	}
	return append(texts, text)
}
