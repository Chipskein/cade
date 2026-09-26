package queryplan

import (
	"time"

	"github.com/chipskein/cade/internal/timeline"
)

// ResolvePeriod turns a question's time reference into dates. A date the
// deterministic parser finds in the question wins over the model's period
// expression, which the model sometimes rewrites; nil means no period.
//
//	days := queryplan.ResolvePeriod("o que fiz ontem?", plan.Period, time.Now())
func ResolvePeriod(question, modelPeriod string, now time.Time) *timeline.DayRange {
	for _, text := range []string{question, modelPeriod} {
		if days, found := timeline.DetectDayRange(text, now); text != "" && found {
			return &days
		}
	}
	return nil
}

// ResolveMode turns a period-less listing into an answer: listing every
// event ever is never what "as mensagens do Marcos" means.
func ResolveMode(mode Mode, days *timeline.DayRange) Mode {
	if mode == ModeList && days == nil {
		return ModeAnswer
	}
	return mode
}
