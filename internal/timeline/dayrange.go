// Package timeline answers "what did I do on these days" across all sources
// (RF3).
package timeline

import (
	"fmt"
	"strings"
	"time"
)

const dayLayout = "2006-01-02"

// DayRange is an inclusive range of calendar days in a time zone.
type DayRange struct {
	First time.Time // midnight of the first day
	Last  time.Time // midnight of the last day
}

// ParseDayRange parses first and (optionally) last as YYYY-MM-DD, "hoje" or
// "ontem", relative to now's time zone. An empty last means a single day.
//
//	days, err := timeline.ParseDayRange("2026-09-01", "2026-09-07", time.Now())
func ParseDayRange(first, last string, now time.Time) (DayRange, error) {
	if last == "" {
		last = first
	}
	firstDay, err := ParseDay(first, now)
	if err != nil {
		return DayRange{}, err
	}
	lastDay, err := ParseDay(last, now)
	if err != nil {
		return DayRange{}, err
	}
	if lastDay.Before(firstDay) {
		return DayRange{}, fmt.Errorf("range end %q is before start %q; expected start <= end", last, first)
	}
	return DayRange{First: firstDay, Last: lastDay}, nil
}

// ParseDay returns midnight (in now's zone) of the named day.
func ParseDay(text string, now time.Time) (time.Time, error) {
	today := midnight(now)
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "hoje", "today":
		return today, nil
	case "ontem", "yesterday":
		return today.AddDate(0, 0, -1), nil
	}
	day, err := time.ParseInLocation(dayLayout, strings.TrimSpace(text), now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q, expected YYYY-MM-DD, \"hoje\" or \"ontem\"", text)
	}
	return day, nil
}

func midnight(moment time.Time) time.Time {
	year, month, day := moment.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, moment.Location())
}

// Start is the first instant of the range.
func (r DayRange) Start() time.Time {
	return r.First
}

// End is the first instant after the range (exclusive bound). AddDate keeps
// this correct across daylight-saving changes, unlike adding 24h.
func (r DayRange) End() time.Time {
	return r.Last.AddDate(0, 0, 1)
}

// IsSingleDay reports whether the range covers exactly one day.
func (r DayRange) IsSingleDay() bool {
	return r.First.Equal(r.Last)
}

// String renders the range for messages, e.g. "2026-09-01 a 2026-09-07".
func (r DayRange) String() string {
	if r.IsSingleDay() {
		return r.First.Format(dayLayout)
	}
	return r.First.Format(dayLayout) + " a " + r.Last.Format(dayLayout)
}
