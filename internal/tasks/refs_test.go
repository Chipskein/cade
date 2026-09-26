package tasks

import (
	"regexp"
	"testing"
)

var (
	proj4mePattern = regexp.MustCompile(`proj4\.me/projects/(\d+)/tasks/(\d+)`)
	jiraPattern    = regexp.MustCompile(`atlassian\.net/browse/([A-Z][A-Z0-9]+-\d+)`)
	testPatterns   = []*regexp.Regexp{proj4mePattern, jiraPattern}
)

func TestTaskRefsAcrossTrackers(t *testing.T) {
	refs := taskRefs(testPatterns, "veja https://app.proj4.me/projects/14/tasks/162 e https://acme.atlassian.net/browse/PROJ-7")
	if len(refs) != 2 || refs[0] != (TaskRef{Key: "14/162", ID: "162"}) || refs[1] != (TaskRef{Key: "PROJ-7", ID: "PROJ-7"}) {
		t.Fatalf("unexpected refs %+v", refs)
	}
}

func TestPRRefsAcrossHosts(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/api/pull/45":                     "acme/api#45",
		"https://gitlab.com/acme/group/api/-/merge_requests/12":   "gitlab.com/acme/group/api#12",
		"https://git.acme.local/team/api/-/merge_requests/3":      "git.acme.local/team/api#3",
		"https://bitbucket.org/acme/api/pull-requests/9":          "acme/api#9",
		"https://dev.azure.com/acme/Proj/_git/api/pullrequest/77": "acme/Proj/_git/api#77",
	}
	for url, expected := range cases {
		refs := prRefs(url)
		if len(refs) != 1 || refs[0].Key() != expected {
			t.Errorf("prRefs(%q) = %+v, expected %s", url, refs, expected)
		}
	}
}

func TestCreateRepoAcrossHosts(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/api/compare/main...fix-162":           "acme/api",
		"https://gitlab.com/acme/api/-/merge_requests/new?source=x":    "gitlab.com/acme/api",
		"https://bitbucket.org/acme/api/pull-requests/new":             "acme/api",
		"https://dev.azure.com/acme/Proj/_git/api/pullrequestcreate?x": "acme/Proj/_git/api",
	}
	for url, expected := range cases {
		if repo, ok := createRepo(url); !ok || repo != expected {
			t.Errorf("createRepo(%q) = %q, expected %q", url, repo, expected)
		}
	}
	if _, ok := createRepo("https://github.com/acme/api/pull/45"); ok {
		t.Error("a PR page is not a creation page")
	}
}

func TestOwnPRTitleAcrossHosts(t *testing.T) {
	cases := map[string]string{
		"Fix CEP sync 162 by bruno · Pull Request #45 · acme/api · GitHub": "Fix CEP sync 162",
		"PROJ-7 login fix (!12) · Merge requests · acme/api · GitLab":      "PROJ-7 login fix",
		"Pull Request 77: fix-cep-162 - Repos":                             "fix-cep-162",
	}
	for title, expected := range cases {
		if got := ownPRTitle(title); got != expected {
			t.Errorf("ownPRTitle(%q) = %q, expected %q", title, got, expected)
		}
	}
}

func TestCitesID(t *testing.T) {
	cited := []struct{ title, id string }{{"fix-cep-162", "162"}, {"162 - ajuste", "162"}, {"PROJ-7 login", "proj-7"}, {"task_162", "162"}}
	for _, c := range cited {
		if !citesID(c.title, c.id) {
			t.Errorf("citesID(%q, %q) = false", c.title, c.id)
		}
	}
	if citesID("release 1620", "162") || citesID("PROJ-70 x", "PROJ-7") {
		t.Error("identifiers must match as whole tokens")
	}
}
