package timeline

import (
	"regexp"
	"time"
)

// English date expressions, so questions can be asked in either language.
// Slash dates follow the configured DateOrder, never the question's
// language: guessing from it would silently pick the wrong day.
var (
	englishMonth = `(jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|june?|july?|aug(?:ust)?|sept?(?:ember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)`
	// "August 12", "Aug 12th, 2025"
	monthFirstPattern = regexp.MustCompile(`\b` + englishMonth + `\.?\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?\b`)
	// "12 August", "12th of Aug 2025"
	dayFirstPattern = regexp.MustCompile(`\b(\d{1,2})(?:st|nd|rd|th)?\s+(?:of\s+)?` + englishMonth + `\.?(?:,?\s+(\d{4}))?\b`)
)

var englishMonthNumbers = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

func englishMonthNumber(name string) int {
	return englishMonthNumbers[name[:3]]
}

// "may" is also the modal verb ("you may 5 times..."), so as a leading
// month it needs a date preposition before it or an ordinal after it.
var (
	datePrepositionBefore = regexp.MustCompile(`\b(on|since|until|from|by|of|in)\s+$`)
	ordinalSuffix         = regexp.MustCompile(`^\d{1,2}(st|nd|rd|th)`)
)

func detectEnglishMonthDate(text string, today time.Time, _ DateOrder) (DayRange, bool) {
	if match := monthFirstPattern.FindStringSubmatchIndex(text); match != nil && !isModalMay(text, match) {
		return singleDay(resolveYear(group(text, match, 3), today), englishMonthNumber(group(text, match, 1)), atoi(group(text, match, 2)), today)
	}
	if match := dayFirstPattern.FindStringSubmatch(text); match != nil {
		return singleDay(resolveYear(match[3], today), englishMonthNumber(match[2]), atoi(match[1]), today)
	}
	return DayRange{}, false
}

func isModalMay(text string, match []int) bool {
	if group(text, match, 1) != "may" {
		return false
	}
	return !datePrepositionBefore.MatchString(text[:match[0]]) && !ordinalSuffix.MatchString(text[match[4]:])
}

// group returns submatch n of an index match, or "" when it did not take part.
func group(text string, match []int, n int) string {
	if match[2*n] < 0 {
		return ""
	}
	return text[match[2*n]:match[2*n+1]]
}

// englishKeywordRanges mirror the Portuguese ones. "last week" is the
// previous calendar week, as "semana passada"; "past week" is 7 days.
var englishKeywordRanges = []keywordRange{
	{regexp.MustCompile(`\bday before yesterday\b`), func(today time.Time) DayRange { return daysAgo(today, 2) }},
	{regexp.MustCompile(`\byesterday\b`), func(today time.Time) DayRange { return daysAgo(today, 1) }},
	{regexp.MustCompile(`\btoday\b`), func(today time.Time) DayRange { return daysAgo(today, 0) }},
	{regexp.MustCompile(`\b(last|previous) week\b`), previousWeek},
	{regexp.MustCompile(`\bthis week\b`), currentWeek},
	{regexp.MustCompile(`\bpast week\b`), func(today time.Time) DayRange { return DayRange{First: today.AddDate(0, 0, -6), Last: today} }},
	{regexp.MustCompile(`\b(last|previous) month\b`), previousMonth},
	{regexp.MustCompile(`\bthis month\b`), currentMonth},
	{regexp.MustCompile(`\blast year\b`), previousYear},
	{regexp.MustCompile(`\bthis year\b`), currentYear},
}
