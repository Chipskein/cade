package timeline

import "testing"

func TestDayPhrase(t *testing.T) {
	cases := map[string]string{
		"O que fiz ONTEM?":                        "ontem",
		"commits da semana passada":               "semana passada",
		"o que houve no dia 12 de agosto":         "12 de agosto",
		"what did I do on Sep 15?":                "sep 15",
		"what did I do the day before yesterday?": "day before yesterday",
		"mensagens dos últimos 3 dias":            "ultimos 3 dias",
		"ontem ou 12/08? a data explícita vence":  "12/08",
	}
	for question, expected := range cases {
		if phrase, ok := DayPhrase(question, phraseNow); !ok || phrase != expected {
			t.Errorf("DayPhrase(%q) = %q (%v), expected %q", question, phrase, ok, expected)
		}
	}
}

func TestDayPhraseWithoutDate(t *testing.T) {
	for _, question := range []string{"o que a Ana me passou?", "you may 5 times retry"} {
		if phrase, ok := DayPhrase(question, phraseNow); ok {
			t.Errorf("DayPhrase(%q) = %q, expected no phrase", question, phrase)
		}
	}
}
