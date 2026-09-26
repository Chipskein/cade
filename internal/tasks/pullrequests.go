package tasks

import (
	"regexp"
	"time"

	"github.com/chipskein/cade/internal/event"
)

// createWindow is how soon after a visit to a PR-creation page the first
// visit to a PR must come for the PR to count as opened by the user: hosts
// redirect to the new PR right after it is created.
const createWindow = 15 * time.Minute

// PullRequest is what the local history reveals about one PR.
type PullRequest struct {
	Ref   PRRef
	Title string
	// OpenedAt is the earliest evidence the user opened it (zero if the
	// user only visited someone else's PR).
	OpenedAt   time.Time
	OpenedByMe bool
	// TaskKeys are tasks linked exactly: a message carrying both links.
	TaskKeys  []string
	firstSeen time.Time
}

// indexPullRequests scans the history for PR visits and PR links.
func indexPullRequests(history []event.Event, taskPatterns []*regexp.Regexp) map[string]*PullRequest {
	index := map[string]*PullRequest{}
	creations := map[string][]time.Time{}
	for _, ev := range history {
		recordCreationPage(ev, creations)
		for _, ref := range prRefs(refText(ev)) {
			recordPRSighting(pullRequest(index, ref), ev, taskPatterns)
		}
	}
	for _, pr := range index {
		markOpenedAfterCreation(pr, creations[pr.Ref.Repo])
	}
	return index
}

func pullRequest(index map[string]*PullRequest, ref PRRef) *PullRequest {
	if index[ref.Key()] == nil {
		index[ref.Key()] = &PullRequest{Ref: ref}
	}
	return index[ref.Key()]
}

func recordCreationPage(ev event.Event, creations map[string][]time.Time) {
	if ev.Source != event.SourceBrowser {
		return
	}
	if repo, ok := createRepo(ev.Metadata["url"]); ok {
		creations[repo] = append(creations[repo], ev.Timestamp)
	}
}

// recordPRSighting notes the first visit and title, and treats a link the
// user sent in a message as proof of opening (people announce their PRs).
func recordPRSighting(pr *PullRequest, ev event.Event, taskPatterns []*regexp.Regexp) {
	if ev.Source == event.SourceBrowser {
		if pr.firstSeen.IsZero() || ev.Timestamp.Before(pr.firstSeen) {
			pr.firstSeen = ev.Timestamp
		}
		if title := ownPRTitle(ev.Metadata["title"]); title != "" {
			pr.Title = title
		}
		return
	}
	if task, ok := singleTask(taskRefs(taskPatterns, ev.Content)); ok {
		pr.TaskKeys = appendUnique(pr.TaskKeys, task)
	}
	if ev.Metadata["sent_by_me"] == "true" {
		pr.markOpened(ev.Timestamp)
	}
}

// singleTask returns the one task a message cites. A message listing
// several tasks and PRs (a release note, a review queue) says nothing about
// which PR belongs to which task; one task with several PRs is common (one
// PR per repository) and links them all.
func singleTask(refs []TaskRef) (string, bool) {
	var keys []string
	for _, ref := range refs {
		keys = appendUnique(keys, ref.Key)
	}
	if len(keys) != 1 {
		return "", false
	}
	return keys[0], true
}

func markOpenedAfterCreation(pr *PullRequest, creations []time.Time) {
	for _, created := range creations {
		gap := pr.firstSeen.Sub(created)
		if !pr.firstSeen.IsZero() && gap >= 0 && gap <= createWindow {
			pr.markOpened(pr.firstSeen)
			return
		}
	}
}

func (pr *PullRequest) markOpened(at time.Time) {
	if !pr.OpenedByMe || at.Before(pr.OpenedAt) {
		pr.OpenedAt = at
	}
	pr.OpenedByMe = true
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
