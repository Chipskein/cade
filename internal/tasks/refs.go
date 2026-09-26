// Package tasks reconstructs which tasks the user worked on in a period,
// which ones were finished (a pull request was opened) and roughly how long
// each took — all from events already stored locally. Tasks and pull
// requests are recognized by their URLs in browser visits and messages.
package tasks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chipskein/cade/internal/event"
)

// TaskRef identifies a task by the capture groups of a task URL pattern;
// ID is the last group, the identifier people write in branch names and
// PR titles ("162", "PROJ-123").
type TaskRef struct {
	Key string
	ID  string
}

// PRRef identifies a pull (or merge) request on a code host.
type PRRef struct {
	Repo   string
	Number int
}

// Key is the display form "org/repo#45".
func (r PRRef) Key() string {
	return fmt.Sprintf("%s#%d", r.Repo, r.Number)
}

// codeHost describes where a host's PR pages and PR-creation pages live.
// Both patterns capture the repository first; pr also captures the number.
type codeHost struct {
	pr     *regexp.Regexp
	create *regexp.Regexp
}

// codeHosts cover the common hosts; GitLab's "/-/" path segment also
// matches self-hosted instances.
var codeHosts = []codeHost{
	{regexp.MustCompile(`https://github\.com/([\w.-]+/[\w.-]+)/pull/(\d+)`),
		regexp.MustCompile(`^https://github\.com/([\w.-]+/[\w.-]+)/compare/`)},
	{regexp.MustCompile(`https?://([\w.-]+(?:/[\w.-]+)+?)/-/merge_requests/(\d+)`),
		regexp.MustCompile(`^https?://([\w.-]+(?:/[\w.-]+)+?)/-/merge_requests/new`)},
	{regexp.MustCompile(`https://bitbucket\.org/([\w.-]+/[\w.-]+)/pull-requests/(\d+)`),
		regexp.MustCompile(`^https://bitbucket\.org/([\w.-]+/[\w.-]+)/pull-requests/new`)},
	{regexp.MustCompile(`https://dev\.azure\.com/([\w.-]+/[\w.%-]+/_git/[\w.%-]+)/pullrequest/(\d+)`),
		regexp.MustCompile(`^https://dev\.azure\.com/([\w.-]+/[\w.%-]+/_git/[\w.%-]+)/pullrequestcreate`)},
}

// Page titles decorate the PR title differently per host:
// "Title by user · Pull Request #45 · org/repo · GitHub",
// "Title (!45) · Merge requests · group/project · GitLab",
// "Pull Request 45: Title - Repos".
var (
	prTitleByline = regexp.MustCompile(`\s+by\s+\S+\s+·.*$`)
	prTitleTail   = regexp.MustCompile(`\s+(·|\(!\d+\)|-\s+Repos).*$`)
	prTitleHead   = regexp.MustCompile(`^Pull Request \d+:\s*`)
)

// refText is where an event's links live: the URL of a visit, the text of
// anything else (Teams keeps pasted links as text).
func refText(ev event.Event) string {
	if ev.Source == event.SourceBrowser {
		return ev.Visit().URL
	}
	return ev.Content
}

// taskRefs finds task links in text using every pattern.
func taskRefs(patterns []*regexp.Regexp, text string) []TaskRef {
	var refs []TaskRef
	for _, pattern := range patterns {
		for _, groups := range pattern.FindAllStringSubmatch(text, -1) {
			if len(groups) >= 2 {
				refs = append(refs, TaskRef{Key: strings.Join(groups[1:], "/"), ID: groups[len(groups)-1]})
			}
		}
	}
	return refs
}

func prRefs(text string) []PRRef {
	var refs []PRRef
	for _, host := range codeHosts {
		for _, groups := range host.pr.FindAllStringSubmatch(text, -1) {
			number, _ := strconv.Atoi(groups[2])
			refs = append(refs, PRRef{Repo: groups[1], Number: number})
		}
	}
	return refs
}

// createRepo reports the repository of a PR-creation page ("compare" on
// GitHub, "merge_requests/new" on GitLab...).
func createRepo(url string) (string, bool) {
	for _, host := range codeHosts {
		if match := host.create.FindStringSubmatch(url); match != nil {
			return match[1], true
		}
	}
	return "", false
}

// ownPRTitle strips the host's page-title decoration.
func ownPRTitle(browserTitle string) string {
	title := prTitleHead.ReplaceAllString(browserTitle, "")
	title = prTitleByline.ReplaceAllString(title, "")
	return strings.TrimSpace(prTitleTail.ReplaceAllString(title, ""))
}

// citesID reports whether a PR title contains the task identifier as a
// whole token: "fix-cep-162" cites "162", "PROJ-123 login" cites
// "PROJ-123", but "1620" does not cite "162".
func citesID(title, id string) bool {
	token := regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(id) + `($|[^\p{L}\p{N}])`)
	return token.MatchString(title)
}
