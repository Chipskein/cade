package cli

import (
	"slices"
	"strings"
	"testing"
)

func TestParseChoice(t *testing.T) {
	cases := []struct {
		answer     string
		includeAll bool
		want       []int
		understood bool
	}{
		{"", true, []int{0, 1, 2}, true},
		{"", false, nil, true},
		{"T", false, []int{0, 1, 2}, true},
		{"all", false, []int{0, 1, 2}, true},
		{"nenhum", true, nil, true},
		{"3 1,3", true, []int{2, 0}, true},
		{"4", true, nil, false},
		{"0", true, nil, false},
		{"talvez", true, nil, false},
	}
	for _, c := range cases {
		got, understood := parseChoice(c.answer, 3, c.includeAll)
		if understood != c.understood || !slices.Equal(got, c.want) {
			t.Errorf("parseChoice(%q, %v) = %v %v, want %v %v", c.answer, c.includeAll, got, understood, c.want, c.understood)
		}
	}
}

func TestPickIndexesKeepsOrderGiven(t *testing.T) {
	if got := pickIndexes([]string{"a", "b", "c"}, []int{2, 0}); !slices.Equal(got, []string{"c", "a"}) {
		t.Fatalf("expected [c a], got %v", got)
	}
}

func TestResolvePath(t *testing.T) {
	if got := resolvePath("~/src", "/home/ana"); got != "/home/ana/src" {
		t.Fatalf("expected /home/ana/src, got %q", got)
	}
	if got := resolvePath("/srv/../srv/docs", "/home/ana"); got != "/srv/docs" {
		t.Fatalf("expected a clean absolute path, got %q", got)
	}
}

func TestConfirmDefaultsToNoAndRetriesUnclearAnswers(t *testing.T) {
	var out strings.Builder
	prompt := newInitPrompt(strings.NewReader("talvez\nsim\n\n"), &out, Portuguese)
	if !prompt.confirm("? ") || !strings.Contains(out.String(), "Resposta não entendida.") {
		t.Fatalf("expected a retry and then yes, got %q", out.String())
	}
	emptyAnswer := prompt.confirm("? ")
	endOfInput := prompt.confirm("? ")
	if emptyAnswer || endOfInput {
		t.Fatalf("expected an empty answer and the end of input to mean no, got %v and %v", emptyAnswer, endOfInput)
	}
}
