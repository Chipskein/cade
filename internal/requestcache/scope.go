// Package requestcache is what the readers of the browsers' request caches
// (the HTTP cache and the Cache API) share: the scope of URLs they may
// read and how one cached response becomes a webstore record.
//
// A request cache holds responses of every site the browser opened, so a
// reader only reads the bodies of URLs the scope names, and a record's
// container is the name of the pattern its URL matched: the schema selects
// "discord-messages", whatever channel id the URL carries.
package requestcache

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Wildcard matches any run of characters in a URL pattern, slashes and
// the query included.
const Wildcard = "*"

// URL schemes a pattern may start with; the authority after them is
// literal, so a scope never reaches beyond the origins it names.
var patternSchemes = []string{"https://", "http://"}

// namePattern keeps scope names usable as a schema's records.container.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// Scope is the named URL patterns a request cache reader may read.
//
//	scope, err := requestcache.NewScope(map[string][]string{
//		"discord-messages": {"https://discord.com/api/v*/channels/*/messages*"},
//	})
type Scope struct {
	patterns []namedPattern
}

type namedPattern struct {
	name    string
	pattern string
}

// NewScope validates named, the config's sources.request_cache_urls. An
// empty scope reads nothing.
func NewScope(named map[string][]string) (Scope, error) {
	var scope Scope
	for name, patterns := range named {
		if !namePattern.MatchString(name) {
			return Scope{}, fmt.Errorf("request cache name %q, expected lowercase letters, digits, dashes and underscores", name)
		}
		for _, pattern := range patterns {
			if err := validatePattern(pattern); err != nil {
				return Scope{}, fmt.Errorf("request cache %q: %w", name, err)
			}
			scope.patterns = append(scope.patterns, namedPattern{name: name, pattern: pattern})
		}
	}
	// Map order is random; a URL two names match gets the same one always.
	slices.SortFunc(scope.patterns, func(a, b namedPattern) int { return strings.Compare(a.name+a.pattern, b.name+b.pattern) })
	return scope, nil
}

func validatePattern(pattern string) error {
	scheme := slices.IndexFunc(patternSchemes, func(s string) bool { return strings.HasPrefix(pattern, s) })
	if scheme < 0 {
		return fmt.Errorf("URL pattern %q, expected it to start with %s", pattern, strings.Join(patternSchemes, " or "))
	}
	literal, _, _ := strings.Cut(pattern[len(patternSchemes[scheme]):], Wildcard)
	if slash := strings.Index(literal, "/"); slash <= 0 {
		return fmt.Errorf("URL pattern %q, expected a literal host followed by / before any %s", pattern, Wildcard)
	}
	return nil
}

// Empty reports whether the scope names no URL: a reader reads nothing.
func (s Scope) Empty() bool {
	return len(s.patterns) == 0
}

// Match names the pattern url matches, the record's container.
//
//	name, ok := scope.Match("https://discord.com/api/v9/channels/1/messages?limit=50") // "discord-messages", true
func (s Scope) Match(url string) (string, bool) {
	for _, named := range s.patterns {
		if globMatches(named.pattern, url) {
			return named.name, true
		}
	}
	return "", false
}

// globMatches reports whether url is pattern with each Wildcard replaced
// by some run of characters.
func globMatches(pattern, url string) bool {
	parts := strings.Split(pattern, Wildcard)
	rest, found := strings.CutPrefix(url, parts[0])
	if !found {
		return false
	}
	last := len(parts) - 1
	if last == 0 {
		return rest == ""
	}
	for _, part := range parts[1:last] {
		_, after, found := strings.Cut(rest, part)
		if !found {
			return false
		}
		rest = after
	}
	return strings.HasSuffix(rest, parts[last])
}
