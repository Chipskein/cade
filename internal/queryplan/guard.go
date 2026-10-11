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

// An image filter needs a word for images, and widening by the graph a word
// for "others"; without them the model's guess would change how an ordinary
// question is searched.
var (
	modalityCues = regexp.MustCompile(`\b(imagem|imagens|fotos?|prints?|capturas?|screenshots?|images?|photos?|pictures?)\b`)
	relatedCues  = regexp.MustCompile(`\b(quem mais|o que mais|outr[ao]s|who else|what else|others?)\b`)
)

// guardPlan drops a mode, status, direction, period, modality, entity or
// widening the question does not support.
func guardPlan(plan Plan, question string) Plan {
	text := textnorm.Fold(question)
	plan = guardTaskReport(plan, text)
	plan = guardGraph(plan, text)
	if !directionSupported(plan.Criteria, text) {
		plan.Criteria.Direction = listing.AnyDirection
	}
	if plan.Period != "" && !strings.Contains(text, textnorm.Fold(plan.Period)) {
		plan.Period = ""
	}
	return plan
}

// guardGraph keeps entities only beside a modality and only when the
// question names them, and widening only from someone named.
func guardGraph(plan Plan, text string) Plan {
	if !modalityCues.MatchString(text) {
		plan.Modality = ""
	}
	plan.Entities = entitiesNamedIn(plan, text)
	if !relatedCues.MatchString(text) || len(plan.Criteria.People)+len(plan.Entities) == 0 {
		plan.Related = false
	}
	return plan
}

// entitiesNamedIn keeps the entities whose first word the question has:
// the model may complete "João" to a full name, never invent one.
func entitiesNamedIn(plan Plan, text string) []string {
	if plan.Modality == "" {
		return nil
	}
	words := wordSet(nonWordRunes.ReplaceAllString(text, " "))
	var named []string
	for _, name := range plan.Entities {
		first := strings.Fields(nonWordRunes.ReplaceAllString(textnorm.Fold(name), " "))
		if len(first) > 0 && words[first[0]] {
			named = append(named, name)
		}
	}
	return named
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
