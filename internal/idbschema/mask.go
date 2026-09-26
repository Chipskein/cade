package idbschema

import (
	"regexp"
	"strings"
)

// The schema report is meant to be shared, so anything that could be user
// data is replaced before printing. Object keys are often data themselves
// (message ids, user MRIs like "8:orgid:<guid>", display names).
var (
	guidPattern   = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	emailPattern  = regexp.MustCompile(`[^\s@:]+@[^\s@:]+`)
	digitsPattern = regexp.MustCompile(`\d{6,}`)
	hexPattern    = regexp.MustCompile(`^[0-9a-fA-F]{16,}$`)
	numberPattern = regexp.MustCompile(`^\d+$`)
)

// maxPlainKeyLen is the longest key kept verbatim; real field names are
// short, long keys are almost always ids or tokens.
const maxPlainKeyLen = 32

// maskKey keeps field-name-like keys and replaces data-like ones.
func maskKey(key string) string {
	switch {
	case strings.ContainsAny(key, " \t\n"):
		return "<text>"
	case numberPattern.MatchString(key), hexPattern.MatchString(key), guidPattern.MatchString(key),
		strings.ContainsAny(key, "@:/"), len(key) > maxPlainKeyLen:
		return "<id>"
	}
	return key
}

// maskName keeps the structure of database and store names (e.g.
// "Teams:replychain-manager:react-web-client:<guid>") while hiding ids.
func maskName(name string) string {
	masked := guidPattern.ReplaceAllString(name, "<guid>")
	masked = emailPattern.ReplaceAllString(masked, "<email>")
	return digitsPattern.ReplaceAllString(masked, "<n>")
}
