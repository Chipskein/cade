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

// MaskKey keeps field-name-like keys and replaces data-like ones with
// "<id>" or "<text>"; idbmap paths match masked keys with the same rule.
func MaskKey(key string) string {
	switch {
	case strings.ContainsAny(key, " \t\n"):
		return "<text>"
	case numberPattern.MatchString(key), hexPattern.MatchString(key), guidPattern.MatchString(key),
		strings.ContainsAny(key, "@:/"), len(key) > maxPlainKeyLen:
		return "<id>"
	}
	return key
}

// MaskName keeps the structure of database and store names (e.g.
// "Teams:replychain-manager:react-web-client:<guid>") while hiding ids.
// Also used on the samples idbdiscovery shows the local model.
func MaskName(name string) string {
	masked := guidPattern.ReplaceAllString(name, "<guid>")
	masked = emailPattern.ReplaceAllString(masked, "<email>")
	return digitsPattern.ReplaceAllString(masked, "<n>")
}

// StablePrefix is name up to the first part MaskName would hide: the part
// shared by every user and session, usable to match the database again
// ("Teams:replychain-manager:react-web-client:<guid>" keeps everything
// before the guid).
//
//	idbschema.StablePrefix("Teams:conv:ana@corp.com:v2") == "Teams:conv:"
func StablePrefix(name string) string {
	end := len(name)
	for _, pattern := range []*regexp.Regexp{guidPattern, emailPattern, digitsPattern} {
		if found := pattern.FindStringIndex(name); found != nil && found[0] < end {
			end = found[0]
		}
	}
	return name[:end]
}
