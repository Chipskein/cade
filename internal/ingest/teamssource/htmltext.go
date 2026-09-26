package teamssource

import (
	"html"
	"regexp"
	"strings"
)

// Teams stores message bodies as HTML fragments. A full parser is not
// needed to get searchable text: line-breaking tags become newlines, the
// remaining tags are dropped and entities decoded.
var (
	lineBreakTags   = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>|</h[1-6]>`)
	nonContentBlock = regexp.MustCompile(`(?is)<(style|script)[^>]*>.*?</(style|script)>`)
	anyTag          = regexp.MustCompile(`<[^>]*>`)
	blankRuns       = regexp.MustCompile(`[ \t\x{00a0}]+`)
	emptyLines      = regexp.MustCompile(`\n\s*\n+`)
	spaceBeforeMark = regexp.MustCompile(` +([,.;:!?])`)
)

// Replies embed the quoted message in a blockquote whose author is a
// <strong itemprop="mri">, and @mentions are spans; stripping their tags
// glued names to the text ("Oficina5mas é estranho").
var (
	quoteBlock  = regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`)
	quoteAuthor = regexp.MustCompile(`(?is)<strong[^>]*itemprop="mri"[^>]*>(.*?)</strong>`)
	mentionSpan = regexp.MustCompile(`(?is)<span[^>]*itemtype="[^"]*Mention[^"]*"[^>]*>(.*?)</span>`)
)

// htmlToText converts a Teams message body to plain text. Quoted messages
// go after the reply, so the first line (the timeline headline) is what the
// sender actually wrote.
//
//	htmlToText("<p>Olá <b>time</b></p>") == "Olá time"
func htmlToText(body string) string {
	body, quotes := extractQuotes(body)
	text := plainText(mentionSpan.ReplaceAllString(body, "@$1 "))
	for _, quote := range quotes {
		text += "\n↪ em resposta a " + quote
	}
	return strings.TrimSpace(text)
}

// extractQuotes removes reply blockquotes and returns them as
// "Author: quoted text".
func extractQuotes(body string) (string, []string) {
	var quotes []string
	remaining := quoteBlock.ReplaceAllStringFunc(body, func(block string) string {
		inner := quoteBlock.FindStringSubmatch(block)[1]
		quote := plainText(anyTag.ReplaceAllString(quoteAuthor.ReplaceAllString(inner, "$1: "), " "))
		if quote != "" {
			quotes = append(quotes, strings.ReplaceAll(quote, "\n", " "))
		}
		return ""
	})
	return remaining, quotes
}

func plainText(fragment string) string {
	text := nonContentBlock.ReplaceAllString(fragment, "")
	text = lineBreakTags.ReplaceAllString(text, "\n")
	text = html.UnescapeString(anyTag.ReplaceAllString(text, ""))
	text = blankRuns.ReplaceAllString(text, " ")
	text = spaceBeforeMark.ReplaceAllString(text, "$1")
	text = emptyLines.ReplaceAllString(text, "\n")
	return strings.TrimSpace(text)
}
