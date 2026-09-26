package cli

import (
	"strings"
	"testing"
)

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestLanguageFromEnvFollowsLocalePrecedence(t *testing.T) {
	cases := []struct {
		env      map[string]string
		expected Language
	}{
		{map[string]string{"LANG": "pt_BR.UTF-8"}, Portuguese},
		{map[string]string{"LANG": "en_US.UTF-8"}, English},
		{map[string]string{"LANG": "pt_BR.UTF-8", "LC_MESSAGES": "en_US.UTF-8"}, English},
		{map[string]string{"LC_MESSAGES": "en_US.UTF-8", "LC_ALL": "pt_PT.UTF-8"}, Portuguese},
		{map[string]string{"LANG": "C"}, English},
		{map[string]string{}, English},
	}
	for i, c := range cases {
		if got := LanguageFromEnv(environment(c.env)); got != c.expected {
			t.Errorf("case %d %v: expected %v, got %v", i, c.env, c.expected, got)
		}
	}
}

func TestHelpCommandPrintsUsageInEachLanguage(t *testing.T) {
	world := newFakeWorld()
	code, portuguese, _ := world.run("help")
	world.language = English
	_, english, _ := world.run("help")
	if code != 0 || !strings.Contains(portuguese, "Comandos:") || !strings.Contains(english, "Commands:") || !strings.Contains(english, "reindex") {
		t.Fatalf("expected help on stdout with exit 0 in both languages, got %d\n%s\n%s", code, portuguese, english)
	}
}

func TestGlobalHelpFlagExitsZero(t *testing.T) {
	world := newFakeWorld()
	world.language = English
	code, stdout, _ := world.run("--help")
	if code != 0 || !strings.Contains(stdout, "Usage:") {
		t.Fatalf("expected the usage and exit 0, got %d %q", code, stdout)
	}
}

func TestCommandHelpUsesLanguage(t *testing.T) {
	world := newFakeWorld()
	world.language = English
	code, _, stderr := world.run("ask", "-h")
	if code != 0 || !strings.Contains(stderr, "Usage of cade ask:") || !strings.Contains(stderr, "search one source only") {
		t.Fatalf("expected the English ask flags and exit 0, got %d %q", code, stderr)
	}
	world.language = Portuguese
	if _, _, stderr := world.run("tasks", "-h"); !strings.Contains(stderr, "Uso de cade tasks:") || !strings.Contains(stderr, "também lista tarefas") {
		t.Fatalf("expected the Portuguese tasks flags, got %q", stderr)
	}
}

func TestUnknownCommandInEnglish(t *testing.T) {
	world := newFakeWorld()
	world.language = English
	if code, _, stderr := world.run("frobnicate"); code != 2 || !strings.Contains(stderr, `unknown command "frobnicate"`) {
		t.Fatalf("expected the English unknown-command message, got %d %q", code, stderr)
	}
}

func TestBadFlagIsStillAUsageError(t *testing.T) {
	if code, _, _ := newFakeWorld().run("ask", "--nope", "x"); code != 2 {
		t.Fatalf("expected exit 2 for an unknown flag, got %d", code)
	}
}
