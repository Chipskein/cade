package timeline

import "testing"

func TestDetectDayRangeReadsSlashDatesInOrder(t *testing.T) {
	cases := []struct {
		question string
		order    DateOrder
		expected string
	}{
		{"o que aconteceu em 12/08", DayFirst, "2026-08-12"},
		{"what happened on 12/08", MonthFirst, "2025-12-08"},
		{"what happened on 08/12/2026", MonthFirst, "2026-08-12"},
		{"what happened on 9/20", MonthFirst, "2026-09-20"},
	}
	for _, c := range cases {
		days, ok := DetectDayRange(c.question, phraseNow, c.order)
		if !ok || days.String() != c.expected {
			t.Errorf("DetectDayRange(%q, %v) = %v (%v), expected %s", c.question, c.order, days, ok, c.expected)
		}
	}
}

func TestDetectDayRangeRejectsSlashDatesImpossibleInOrder(t *testing.T) {
	if days, ok := DetectDayRange("o que houve em 20/09", phraseNow, MonthFirst); ok {
		t.Errorf("expected no month 20 in mdy, got %v", days)
	}
	if days, ok := DetectDayRange("what happened on 9/20", phraseNow, DayFirst); ok {
		t.Errorf("expected no month 20 in dmy, got %v", days)
	}
}

func TestDayPhraseFindsSlashDateInOrder(t *testing.T) {
	if phrase, ok := DayPhrase("commits from 9/20", phraseNow, MonthFirst); !ok || phrase != "9/20" {
		t.Errorf("DayPhrase in mdy = %q (%v), expected \"9/20\"", phrase, ok)
	}
}

func TestParseDateOrder(t *testing.T) {
	if order, err := ParseDateOrder("dmy"); err != nil || order != DayFirst {
		t.Errorf("ParseDateOrder(dmy) = %v, %v; expected DayFirst", order, err)
	}
	if order, err := ParseDateOrder("mdy"); err != nil || order != MonthFirst {
		t.Errorf("ParseDateOrder(mdy) = %v, %v; expected MonthFirst", order, err)
	}
	if _, err := ParseDateOrder("ymd"); err == nil {
		t.Error("expected ymd rejected")
	}
}
