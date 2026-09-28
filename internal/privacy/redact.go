// Package privacy removes common credentials before events are stored.
package privacy

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/chipskein/cade/internal/event"
)

var patterns = []struct {
	expression *regexp.Regexp
	label      string
}{
	{regexp.MustCompile(`(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`), "github-token"},
	{regexp.MustCompile(`glpat-[A-Za-z0-9_-]{16,}`), "gitlab-token"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "aws-key"},
	{regexp.MustCompile(`xox[bp]-[A-Za-z0-9-]{10,}`), "slack-token"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`), "jwt"},
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), "private-key"},
}

var secretQueryKeys = []string{"token", "access_token", "id_token", "refresh_token", "code", "state", "sig", "signature", "key", "apikey", "api_key", "password"}

func Text(text string) string {
	for _, pattern := range patterns {
		text = pattern.expression.ReplaceAllString(text, "[redacted:"+pattern.label+"]")
	}
	return text
}

func URL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery == "" {
		return raw
	}
	var retained []string
	for _, pair := range strings.Split(parsed.RawQuery, "&") {
		key, _, _ := strings.Cut(pair, "=")
		decoded, decodeErr := url.QueryUnescape(key)
		lower := strings.ToLower(decoded)
		if decodeErr == nil && !secretParameter(lower) {
			retained = append(retained, pair)
		}
	}
	parsed.RawQuery = strings.Join(retained, "&")
	parsed.ForceQuery = false
	return parsed.String()
}

func secretParameter(key string) bool {
	for _, candidate := range secretQueryKeys {
		if key == candidate {
			return true
		}
	}
	return strings.HasPrefix(key, "x-amz-") || strings.HasPrefix(key, "x-goog-")
}

func Event(input event.Event, redact bool) event.Event {
	metadata := make(event.Metadata, len(input.Metadata))
	for key, value := range input.Metadata {
		metadata[key] = value
	}
	input.Metadata = metadata
	if input.Source == event.SourceBrowser {
		input.Metadata["url"] = URL(input.Metadata["url"])
	}
	if redact {
		input.Content = Text(input.Content)
		for key, value := range input.Metadata {
			input.Metadata[key] = Text(URL(value))
		}
	}
	return input
}
