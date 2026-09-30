package cli

import (
	"strings"

	"github.com/chipskein/cade/internal/timeline"
)

// dateLocaleVariables are read in gettext's order of precedence for dates.
var dateLocaleVariables = []string{"LC_ALL", "LC_TIME", "LANG"}

// DateOrderFromEnv reads "12/08" month first under en_US, as people there
// write it, and day first under every other locale.
//
//	order := cli.DateOrderFromEnv(os.Getenv)
func DateOrderFromEnv(getenv func(string) string) timeline.DateOrder {
	if strings.HasPrefix(firstLocale(getenv, dateLocaleVariables), "en_US") {
		return timeline.MonthFirst
	}
	return timeline.DayFirst
}

// dateOrderFromSetting applies ui.date_order: "dmy" or "mdy" override the
// locale, "auto" keeps it.
func dateOrderFromSetting(setting string, fromLocale timeline.DateOrder) timeline.DateOrder {
	order, err := timeline.ParseDateOrder(setting)
	if err != nil {
		return fromLocale
	}
	return order
}
