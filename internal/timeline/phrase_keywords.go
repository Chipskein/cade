package timeline

import (
	"regexp"
	"time"
)

// keywordRange maps a relative expression to the range it names.
type keywordRange struct {
	pattern *regexp.Regexp
	resolve func(today time.Time) DayRange
}

// Weeks start on Monday. "semana passada" is the previous calendar week;
// "ultima semana" is the last seven days, as it is commonly used.
var keywordRanges = []keywordRange{
	{regexp.MustCompile(`\banteontem\b`), func(today time.Time) DayRange { return daysAgo(today, 2) }},
	{regexp.MustCompile(`\bontem\b`), func(today time.Time) DayRange { return daysAgo(today, 1) }},
	{regexp.MustCompile(`\bhoje\b`), func(today time.Time) DayRange { return daysAgo(today, 0) }},
	{regexp.MustCompile(`\bsemana (passada|anterior)\b`), previousWeek},
	{regexp.MustCompile(`\b(esta|essa|nesta|nessa) semana\b`), currentWeek},
	{regexp.MustCompile(`\bultima semana\b`), func(today time.Time) DayRange { return DayRange{First: today.AddDate(0, 0, -6), Last: today} }},
	{regexp.MustCompile(`\bmes (passado|anterior)\b`), previousMonth},
	{regexp.MustCompile(`\b(este|esse|neste|nesse) mes\b`), currentMonth},
	{regexp.MustCompile(`\bano passado\b`), previousYear},
	{regexp.MustCompile(`\b(este|esse|neste|nesse) ano\b`), currentYear},
}

// allKeywordRanges checks Portuguese first, then English.
var allKeywordRanges = append(append([]keywordRange{}, keywordRanges...), englishKeywordRanges...)

func detectKeyword(text string, today time.Time, _ DateOrder) (DayRange, bool) {
	for _, keyword := range allKeywordRanges {
		if keyword.pattern.MatchString(text) {
			return keyword.resolve(today), true
		}
	}
	return DayRange{}, false
}

func daysAgo(today time.Time, days int) DayRange {
	day := today.AddDate(0, 0, -days)
	return DayRange{First: day, Last: day}
}

func weekStart(today time.Time) time.Time {
	sinceMonday := (int(today.Weekday()) + 6) % 7
	return today.AddDate(0, 0, -sinceMonday)
}

func currentWeek(today time.Time) DayRange {
	return DayRange{First: weekStart(today), Last: today}
}

func previousWeek(today time.Time) DayRange {
	monday := weekStart(today).AddDate(0, 0, -7)
	return DayRange{First: monday, Last: monday.AddDate(0, 0, 6)}
}

func currentMonth(today time.Time) DayRange {
	return DayRange{First: today.AddDate(0, 0, 1-today.Day()), Last: today}
}

func previousMonth(today time.Time) DayRange {
	firstOfThisMonth := today.AddDate(0, 0, 1-today.Day())
	return DayRange{First: firstOfThisMonth.AddDate(0, -1, 0), Last: firstOfThisMonth.AddDate(0, 0, -1)}
}

func currentYear(today time.Time) DayRange {
	return DayRange{First: time.Date(today.Year(), 1, 1, 0, 0, 0, 0, today.Location()), Last: today}
}

func previousYear(today time.Time) DayRange {
	first := time.Date(today.Year()-1, 1, 1, 0, 0, 0, 0, today.Location())
	return DayRange{First: first, Last: first.AddDate(1, 0, -1)}
}
