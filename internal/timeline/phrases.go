package timeline

import (
	"regexp"
	"strconv"
	"time"

	"github.com/chipskein/cade/internal/textnorm"
)

// dayDetector returns the range a question refers to, if its phrase
// matches; only slash dates depend on order.
type dayDetector func(text string, today time.Time, order DateOrder) (DayRange, bool)

// DetectDayRange finds a date reference in a Portuguese question ("ontem",
// "semana passada", "12/08"...) relative to now. Without it, "o que fiz
// ontem?" is matched by meaning only and finds messages that merely contain
// the word "ontem", from any date. Explicit dates are checked first; order
// says whether "12/08" is 12 August or December 8.
//
//	days, ok := timeline.DetectDayRange("o que a Ana me passou ontem?", time.Now(), timeline.DayFirst)
func DetectDayRange(question string, now time.Time, order DateOrder) (DayRange, bool) {
	text := textnorm.Fold(question)
	today := midnight(now)
	for _, detect := range dayDetectors {
		if days, ok := detect(text, today, order); ok {
			return days, true
		}
	}
	return DayRange{}, false
}

var dayDetectors = []dayDetector{detectISODate, detectSlashDate, detectMonthNameDate, detectEnglishMonthDate, detectLastNDays, detectKeyword}

var (
	isoDatePattern       = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	slashDatePattern     = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})(?:/(\d{4}|\d{2}))?\b`)
	monthNameDatePattern = regexp.MustCompile(`\b(\d{1,2}) de (janeiro|fevereiro|marco|abril|maio|junho|julho|agosto|setembro|outubro|novembro|dezembro)(?: de (\d{4}))?\b`)
	lastNDaysPattern     = regexp.MustCompile(`\b(?:ultimos|last|past) (\d{1,3}) (?:dias|days)\b`)
)

var monthNumbers = map[string]int{"janeiro": 1, "fevereiro": 2, "marco": 3, "abril": 4, "maio": 5, "junho": 6,
	"julho": 7, "agosto": 8, "setembro": 9, "outubro": 10, "novembro": 11, "dezembro": 12}

func detectISODate(text string, today time.Time, _ DateOrder) (DayRange, bool) {
	match := isoDatePattern.FindStringSubmatch(text)
	if match == nil {
		return DayRange{}, false
	}
	return singleDay(atoi(match[1]), atoi(match[2]), atoi(match[3]), today)
}

func detectSlashDate(text string, today time.Time, order DateOrder) (DayRange, bool) {
	match := slashDatePattern.FindStringSubmatch(text)
	if match == nil {
		return DayRange{}, false
	}
	day, month := order.dayAndMonth(atoi(match[1]), atoi(match[2]))
	return singleDay(resolveYear(match[3], today), month, day, today)
}

func detectMonthNameDate(text string, today time.Time, _ DateOrder) (DayRange, bool) {
	match := monthNameDatePattern.FindStringSubmatch(text)
	if match == nil {
		return DayRange{}, false
	}
	return singleDay(resolveYear(match[3], today), monthNumbers[match[2]], atoi(match[1]), today)
}

func detectLastNDays(text string, today time.Time, _ DateOrder) (DayRange, bool) {
	match := lastNDaysPattern.FindStringSubmatch(text)
	if match == nil || atoi(match[1]) < 1 {
		return DayRange{}, false
	}
	return DayRange{First: today.AddDate(0, 0, 1-atoi(match[1])), Last: today}, true
}

func atoi(digits string) int {
	number, _ := strconv.Atoi(digits)
	return number
}

// resolveYear expands a missing or two-digit year. Without a year the date
// is assumed to be the most recent one, so "12/08" never means the future
// (see singleDay).
func resolveYear(digits string, today time.Time) int {
	switch len(digits) {
	case 0:
		return 0
	case 2:
		return 2000 + atoi(digits)
	}
	return atoi(digits)
}

// singleDay builds a one-day range, rejecting impossible dates (31/02). A
// zero year means "this year, or last year if that is still ahead".
func singleDay(year, month, day int, today time.Time) (DayRange, bool) {
	inferYear := year == 0
	if inferYear {
		year = today.Year()
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, today.Location())
	if date.Day() != day || int(date.Month()) != month {
		return DayRange{}, false
	}
	if inferYear && date.After(today) {
		date = date.AddDate(-1, 0, 0)
	}
	return DayRange{First: date, Last: date}, true
}
