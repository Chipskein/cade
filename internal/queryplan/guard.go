package queryplan

import (
	"regexp"
	"strings"

	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/textnorm"
)

// The 3B model sometimes invents restrictions: "essa semana" came back as
// "semana passada", "conversas com o Everton" as "enviadas". A wrong filter
// hides the right events, which is worse than no filter, so these checks
// only ever remove what the question does not support.
var (
	receivedVerbs = regexp.MustCompile(`\b(recebi|recebid[ao]s?|me (passou|passaram|mandou|mandaram|enviou|enviaram|pediu|pediram|falou|falaram|disse|disseram|perguntou)|received|(sent|send|told|tell|asked|ask|gave|give|passed|pass|messaged|message|wrote|write) me)\b`)
	sentVerbs     = regexp.MustCompile(`\b(mandei|enviei|pedi|falei|perguntei|respondi|i (sent|send|told|tell|asked|ask|messaged|message|replied|reply|wrote|write|answered|answer))\b`)
)

// English questions with "did" use the base form ("what did Ana send me",
// "what did I tell"), so both forms are listed.

// Prepositions that point at a plan person: "da Ana" (received), "pro
// Willian" (sent). Without the person check, "resumo do que..." would count.
var (
	fromPrepositions = []string{"de", "da", "do", "das", "dos", "from"}
	toPrepositions   = []string{"para", "pra", "pro", "para a", "para o", "to"}
)

// Task reports need the question to be about tasks, and a status filter
// needs a word for it; "não finalizei" counts as unfinished.
var (
	taskCues       = regexp.MustCompile(`\b(tarefas?|tasks?|tickets?|cards?|demandas?|issues?)\b`)
	doneCues       = regexp.MustCompile(`(finaliz|conclu|termin|entreg|finish|complet|\bdone\b|\bclosed\b)`)
	inProgressCues = regexp.MustCompile(`(andamento|pendente|abert|faltando|progress|pending|\bopen\b|unfinished|nao (finaliz|conclu|termin))`)
	prOpenedCues   = regexp.MustCompile(`\b(pr|pull request)\s+(aberto|aberta|opened)\b`)
)

// guardPlan drops a mode, status, direction or period the question does
// not support.
func guardPlan(plan Plan, question string) Plan {
	text := textnorm.Fold(question)
	plan = guardTaskReport(plan, text)
	if !directionSupported(plan.Criteria, text) {
		plan.Criteria.Direction = listing.AnyDirection
	}
	if plan.Period != "" && !strings.Contains(text, textnorm.Fold(plan.Period)) {
		plan.Period = ""
	}
	return plan
}

func guardTaskReport(plan Plan, text string) Plan {
	if plan.Mode == ModeTasks && !taskCues.MatchString(text) {
		plan.Mode = ModeAnswer
	}
	if plan.Mode != ModeTasks || !statusSupported(plan.TaskStatus, text) {
		plan.TaskStatus = AnyStatus
	}
	return plan
}

func statusSupported(status TaskStatus, text string) bool {
	switch status {
	case OnlyDone:
		return (doneCues.MatchString(text) || prOpenedCues.MatchString(text)) && (!inProgressCues.MatchString(text) || prOpenedCues.MatchString(text))
	case OnlyInProgress:
		return inProgressCues.MatchString(text)
	}
	return true
}

func directionSupported(criteria listing.Criteria, text string) bool {
	switch criteria.Direction {
	case listing.Received:
		return receivedVerbs.MatchString(text) || mentionsPersonAfter(text, fromPrepositions, criteria.People)
	case listing.Sent:
		return sentVerbs.MatchString(text) || mentionsPersonAfter(text, toPrepositions, criteria.People)
	}
	return true
}

func mentionsPersonAfter(text string, prepositions, people []string) bool {
	padded := " " + text + " "
	for _, person := range people {
		words := strings.Fields(textnorm.Fold(person))
		if len(words) == 0 {
			continue
		}
		for _, preposition := range prepositions {
			if strings.Contains(padded, " "+preposition+" "+words[0]) {
				return true
			}
		}
	}
	return false
}
