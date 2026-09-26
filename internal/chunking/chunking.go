// Package chunking splits long event text into pieces the embedder can
// read whole. The embedding model sees at most 512 tokens; a 20 KB note
// embedded as one vector was cut after its first ~2,000 characters, so an
// answer in its second half could never be found.
package chunking

import (
	"strings"
	"unicode/utf8"
)

// MaxChars keeps a chunk well under 512 tokens (Portuguese and code run at
// roughly 3 to 4 characters per token); Overlap repeats the end of one
// chunk at the start of the next, so a sentence cut at a border is whole
// in one of them.
const (
	MaxChars = 1200
	Overlap  = 120
)

// Span is a chunk as byte offsets into the text: [Start, End).
type Span struct {
	Start int
	End   int
}

// Split returns the spans of text: one for short text, else pieces cut at
// Markdown headings, then paragraphs, then lines, then spaces, each at most
// MaxChars, overlapping by up to Overlap. Offsets fall on UTF-8 boundaries.
//
//	for _, span := range chunking.Split(note) { embed(note[span.Start:span.End]) }
func Split(text string) []Span {
	if len(text) <= MaxChars {
		return []Span{{Start: 0, End: len(text)}}
	}
	var spans []Span
	for start := 0; start < len(text); {
		end := cutPoint(text, start)
		spans = append(spans, Span{Start: start, End: end})
		if end == len(text) {
			break
		}
		start = nextStart(text, start, end)
	}
	return spans
}

// separators are tried in order, from the strongest boundary.
var separators = []string{"\n#", "\n\n", "\n", " "}

// cutPoint ends a chunk at the last strong boundary within MaxChars of
// start, preferring a cut past the first half so chunks are not tiny.
func cutPoint(text string, start int) int {
	limit := start + MaxChars
	if limit >= len(text) {
		return len(text)
	}
	window := text[start:limit]
	for _, separator := range separators {
		if cut := strings.LastIndex(window, separator); cut > MaxChars/2 {
			return start + cut + boundaryWidth(separator)
		}
	}
	return runeStart(text, limit)
}

// boundaryWidth keeps a heading's "#" with the next chunk and the rest of
// the separator (newlines, space) with this one.
func boundaryWidth(separator string) int {
	if separator == "\n#" {
		return 1
	}
	return len(separator)
}

// nextStart begins the next chunk up to Overlap before end, at a space so
// no word is split, and always after start so Split terminates.
func nextStart(text string, start, end int) int {
	from := runeStart(text, max(end-Overlap, start+1))
	if space := strings.IndexByte(text[from:end], ' '); space >= 0 {
		return from + space + 1
	}
	return end
}

// runeStart moves offset back to the start of the rune it falls in.
func runeStart(text string, offset int) int {
	for offset > 0 && offset < len(text) && !utf8.RuneStart(text[offset]) {
		offset--
	}
	return offset
}
