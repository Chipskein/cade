package timeline

import (
	"regexp"
	"time"

	"github.com/chipskein/cade/internal/textnorm"
)

// dayPhrasePatterns are the expressions DetectDayRange reads, in the order
// it tries them.
var dayPhrasePatterns = append([]*regexp.Regexp{isoDatePattern, slashDatePattern, monthNameDatePattern,
	monthFirstPattern, dayFirstPattern, lastNDaysPattern}, keywordPatterns(allKeywordRanges)...)

func keywordPatterns(ranges []keywordRange) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, len(ranges))
	for i, keyword := range ranges {
		patterns[i] = keyword.pattern
	}
	return patterns
}

// DayPhrase returns, folded, the expression DetectDayRange resolves in
// question, so a caller can tell what else the question says. A phrase
// only counts when it names the same days on its own, which skips "may"
// used as a verb.
//
//	phrase, ok := timeline.DayPhrase("commits da semana passada", time.Now()) // "semana passada", true
func DayPhrase(question string, now time.Time) (string, bool) {
	days, found := DetectDayRange(question, now)
	if !found {
		return "", false
	}
	text := textnorm.Fold(question)
	for _, pattern := range dayPhrasePatterns {
		if phrase, ok := phraseNaming(pattern.FindAllString(text, -1), days, now); ok {
			return phrase, true
		}
	}
	return "", false
}

func phraseNaming(phrases []string, days DayRange, now time.Time) (string, bool) {
	for _, phrase := range phrases {
		if phraseDays, ok := DetectDayRange(phrase, now); ok && phraseDays.String() == days.String() {
			return phrase, true
		}
	}
	return "", false
}
