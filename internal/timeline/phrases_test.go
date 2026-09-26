package timeline

import (
	"testing"
	"time"
)

// Saturday, 2026-09-26.
var phraseNow = time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC)

func TestDetectDayRange(t *testing.T) {
	cases := map[string]string{
		"O que a Ana me passou ontem?":           "2026-09-25",
		"o que fiz anteontem":                    "2026-09-24",
		"reuniões de HOJE":                       "2026-09-26",
		"o que pesquisei na semana passada":      "2026-09-14 a 2026-09-20",
		"o que fiz nesta semana":                 "2026-09-21 a 2026-09-26",
		"commits da última semana":               "2026-09-20 a 2026-09-26",
		"mensagens dos últimos 3 dias":           "2026-09-24 a 2026-09-26",
		"o que mudou no mês passado":             "2026-08-01 a 2026-08-31",
		"o que fiz este mês":                     "2026-09-01 a 2026-09-26",
		"projetos do ano passado":                "2025-01-01 a 2025-12-31",
		"o que aconteceu em 12/08":               "2026-08-12",
		"o que aconteceu em 12/08/25":            "2025-08-12",
		"o que houve no dia 12 de agosto":        "2026-08-12",
		"o que houve em 2026-08-12":              "2026-08-12",
		"o que combinamos em 30/12":              "2025-12-30",
		"ontem ou 12/08? a data explícita vence": "2026-08-12",
	}
	for question, expected := range cases {
		days, ok := DetectDayRange(question, phraseNow)
		if !ok || days.String() != expected {
			t.Errorf("DetectDayRange(%q) = %v (%v), expected %s", question, days, ok, expected)
		}
	}
}

func TestDetectDayRangeWithoutDate(t *testing.T) {
	for _, question := range []string{"o que a Ana me passou?", "arquivo 31/02 inválido", "últimos 0 dias", "anteontemzinho"} {
		if days, ok := DetectDayRange(question, phraseNow); ok {
			t.Errorf("DetectDayRange(%q) = %v, expected no date", question, days)
		}
	}
}

func TestDetectDayRangeUsesLocalMidnight(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)
	days, _ := DetectDayRange("ontem", time.Date(2026, 9, 26, 1, 0, 0, 0, brt))
	if !days.Start().Equal(time.Date(2026, 9, 25, 0, 0, 0, 0, brt)) {
		t.Fatalf("expected BRT midnight of the 25th, got %s", days.Start())
	}
}

func TestResolveYear(t *testing.T) {
	if resolveYear("", phraseNow) != 0 || resolveYear("25", phraseNow) != 2025 || resolveYear("2024", phraseNow) != 2024 {
		t.Fatal("expected 0 for missing, 20xx for two digits, as-is for four")
	}
}

func TestWeekStartIsMonday(t *testing.T) {
	sunday := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	if got := weekStart(sunday); got.Day() != 21 || got.Weekday() != time.Monday {
		t.Fatalf("expected Monday the 21st, got %s", got)
	}
}

func TestDetectDayRangeEnglish(t *testing.T) {
	cases := map[string]string{
		"what did Ana send me yesterday?":        "2026-09-25",
		"messages from the day before yesterday": "2026-09-24",
		"commits I made today":                   "2026-09-26",
		"what did I search last week":            "2026-09-14 a 2026-09-20",
		"what did I do this week":                "2026-09-21 a 2026-09-26",
		"pages visited in the past week":         "2026-09-20 a 2026-09-26",
		"messages from the last 3 days":          "2026-09-24 a 2026-09-26",
		"what changed last month":                "2026-08-01 a 2026-08-31",
		"projects from last year":                "2025-01-01 a 2025-12-31",
		"what happened on August 12":             "2026-08-12",
		"what happened on Aug 12th, 2025":        "2025-08-12",
		"what happened on the 12th of August":    "2026-08-12",
		"what did we agree on Dec 30":            "2025-12-30",
	}
	for question, expected := range cases {
		days, ok := DetectDayRange(question, phraseNow)
		if !ok || days.String() != expected {
			t.Errorf("DetectDayRange(%q) = %v (%v), expected %s", question, days, ok, expected)
		}
	}
}

func TestDetectDayRangeEnglishWithoutDate(t *testing.T) {
	for _, question := range []string{"what did Ana send me?", "you may 5 times check", "February 30"} {
		if days, ok := DetectDayRange(question, phraseNow); ok {
			t.Errorf("DetectDayRange(%q) = %v, expected no date", question, days)
		}
	}
}

func TestEnglishMonthNumber(t *testing.T) {
	if englishMonthNumber("august") != 8 || englishMonthNumber("sept") != 9 || englishMonthNumber("dec") != 12 {
		t.Fatal("unexpected month numbers")
	}
}

func TestMayAsMonthNeedsDateContext(t *testing.T) {
	onMay, _ := DetectDayRange("what happened on May 12?", phraseNow)
	ordinal, _ := DetectDayRange("May 3rd meeting notes", phraseNow)
	if onMay.String() != "2026-05-12" || ordinal.String() != "2026-05-03" {
		t.Fatalf("expected May dates with context, got %v / %v", onMay, ordinal)
	}
}
