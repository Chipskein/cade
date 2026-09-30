package cli

import (
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/timeline"
)

func TestDateOrderFromEnvIsMonthFirstOnlyForUnitedStates(t *testing.T) {
	cases := []struct {
		env      map[string]string
		expected timeline.DateOrder
	}{
		{map[string]string{"LANG": "en_US.UTF-8"}, timeline.MonthFirst},
		{map[string]string{"LANG": "en_GB.UTF-8"}, timeline.DayFirst},
		{map[string]string{"LANG": "pt_BR.UTF-8"}, timeline.DayFirst},
		{map[string]string{"LANG": "en_US.UTF-8", "LC_TIME": "pt_BR.UTF-8"}, timeline.DayFirst},
		{map[string]string{"LC_TIME": "pt_BR.UTF-8", "LC_ALL": "en_US.UTF-8"}, timeline.MonthFirst},
		{map[string]string{"LANG": "C"}, timeline.DayFirst},
		{map[string]string{}, timeline.DayFirst},
	}
	for i, c := range cases {
		if got := DateOrderFromEnv(environment(c.env)); got != c.expected {
			t.Errorf("case %d %v: expected %v, got %v", i, c.env, c.expected, got)
		}
	}
}

func TestDateOrderFromSettingOverridesLocale(t *testing.T) {
	if dateOrderFromSetting("dmy", timeline.MonthFirst) != timeline.DayFirst || dateOrderFromSetting("mdy", timeline.DayFirst) != timeline.MonthFirst {
		t.Fatal("expected dmy and mdy to override the locale")
	}
	if dateOrderFromSetting("auto", timeline.MonthFirst) != timeline.MonthFirst {
		t.Fatal("expected auto to keep the locale's order")
	}
}

func TestAskReadsNumericDateInEachOrder(t *testing.T) {
	cases := map[string]string{"dmy": "2026-08-12", "mdy": "2025-12-08"}
	for setting, expected := range cases {
		world := newFakeWorld()
		world.cfg.UI.DateOrder = setting
		_, _, stderr := world.run("ask", "o que fiz em 12/08?")
		if !strings.Contains(stderr, "Entendi: responder · "+expected) {
			t.Errorf("date_order %s: expected %s, got %q", setting, expected, stderr)
		}
	}
}

func TestAskDateOrderFollowsLocaleOnAuto(t *testing.T) {
	world := newFakeWorld()
	world.dateOrder = timeline.MonthFirst
	if _, _, stderr := world.run("ask", "o que fiz em 12/08?"); !strings.Contains(stderr, "2025-12-08") {
		t.Fatalf("expected the en_US locale to read 12/08 as December 8, got %q", stderr)
	}
}
