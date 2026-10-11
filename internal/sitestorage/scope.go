// Package sitestorage is what the readers of a site's own storages
// (localStorage and the Origin Private File System) share: the origins
// they may read and how a stored text becomes a value tree.
//
// A profile's localStorage holds every site the browser opened, session
// tokens among them, so a reader only reads the origins the scope names.
package sitestorage

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// originSchemes are the schemes an origin may have; a site's storage is
// keyed by scheme, host and port.
var originSchemes = []string{"https", "http"}

// ErrEmptyScope stops a reader before it reads: with no origin configured
// nothing is read.
var ErrEmptyScope = errors.New("no origin in sources.storage_origins, expected at least one: a profile's localStorage and OPFS hold every site, session tokens among them, so only configured origins are read")

// Scope is the origins a site storage reader may read.
//
//	scope, err := sitestorage.NewScope([]string{"https://chatgpt.com"})
type Scope struct {
	origins []string
}

// NewScope validates origins, the config's sources.storage_origins. An
// empty scope reads nothing.
func NewScope(origins []string) (Scope, error) {
	var scope Scope
	for _, origin := range origins {
		if err := validateOrigin(origin); err != nil {
			return Scope{}, err
		}
		scope.origins = append(scope.origins, strings.ToLower(origin))
	}
	return scope, nil
}

func validateOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil || !slices.Contains(originSchemes, parsed.Scheme) || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("storage origin %q, expected scheme://host[:port] with scheme %s", origin, strings.Join(originSchemes, " or "))
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("storage origin %q, expected no path, query or fragment after the host", origin)
	}
	return nil
}

// Empty reports whether the scope names no origin: a reader reads nothing.
func (s Scope) Empty() bool {
	return len(s.origins) == 0
}

// Allows reports whether origin, as a browser names it, is in the scope.
//
//	scope.Allows("https://chatgpt.com") // true
func (s Scope) Allows(origin string) bool {
	return slices.Contains(s.origins, strings.ToLower(origin))
}

// Refusal reports origin, found at location, as outside the scope: a
// configured location of an origin nobody allowed is an error, not an
// empty read.
func Refusal(origin, location string) error {
	return fmt.Errorf("origin %q of %q is not in sources.storage_origins, expected it listed there to be read", origin, location)
}
