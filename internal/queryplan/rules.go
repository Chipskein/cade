package queryplan

import (
	"regexp"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/textnorm"
	"github.com/chipskein/cade/internal/timeline"
)

// Many questions are only a period, a source and generic words ("liste os
// commits de ontem", "o que fiz hoje?"). Reading those needs no model, which
// takes ~1.3 s on a GPU and much longer on a CPU. A question with any word
// not listed here (a name, a topic, a number) goes to the model: the rules
// never guess.
var (
	ruleSourceNouns = map[string]event.Source{
		"commit": event.SourceGit, "commits": event.SourceGit,
		"pagina": event.SourceBrowser, "paginas": event.SourceBrowser, "site": event.SourceBrowser, "sites": event.SourceBrowser,
		"pesquisas": event.SourceBrowser, "navegador": event.SourceBrowser, "page": event.SourceBrowser, "pages": event.SourceBrowser,
		"arquivo": event.SourceFile, "arquivos": event.SourceFile, "file": event.SourceFile, "files": event.SourceFile,
		"mensagem": event.SourceTeams, "mensagens": event.SourceTeams, "conversa": event.SourceTeams, "conversas": event.SourceTeams,
		"chat": event.SourceTeams, "chats": event.SourceTeams, "message": event.SourceTeams, "messages": event.SourceTeams,
		"conversation": event.SourceTeams, "conversations": event.SourceTeams,
	}
	ruleDirectionWords = map[string]listing.Direction{
		"recebi": listing.Received, "recebidas": listing.Received, "received": listing.Received,
		"enviei": listing.Sent, "mandei": listing.Sent, "enviadas": listing.Sent, "sent": listing.Sent,
	}
	ruleTaskNouns    = wordSet("tarefa tarefas task tasks ticket tickets card cards demanda demandas issue issues")
	ruleSummaryWords = wordSet("resuma resumo resumir summarize summary")
	ruleListAllWords = wordSet("tudo everything timeline")
	// ruleGenericWords carry no filter: articles, pronouns, question words
	// and verbs of doing or viewing. Status words are read from the whole
	// text by the same cues that guard the model's plan.
	ruleGenericWords = wordSet("o a os as um uma que quais qual eu meu meus minha minhas de do da dos das em no na nos nas " +
		"fiz fizemos trabalhei aconteceu estao ainda ficaram liste listar mostre abri visitei acessei editei alterei " +
		"modifiquei modificados modificadas todas todos dia " +
		"finalizei finalizadas conclui concluidas terminei entreguei pendentes pendente andamento abertas abertos nao " +
		"what which did i do work worked on in the my from of show list me happened are is still all day " +
		"finish finished complete completed done closed open pending progress edit edited changed modified visited opened")
)

// ruleAskingCues make a source noun ambiguous: "o que tem nas mensagens de
// ontem?" may want an answer or the list.
var ruleAskingCues = regexp.MustCompile(`\b(o que|what)\b`)

var nonWordRunes = regexp.MustCompile(`[^a-z0-9]+`)

// ruleAnchor resolves periods only to compare them; any fixed day works.
var ruleAnchor = time.Date(2000, 6, 15, 12, 0, 0, 0, time.UTC)

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// PlanByRules reads question without the model when every word is a
// period, a source, a mode or status cue, or a generic word; false sends
// the question to the model.
//
//	plan, ok := queryplan.PlanByRules("liste os commits de ontem") // listar, git, "ontem"
func PlanByRules(question string) (Plan, bool) {
	text := textnorm.Fold(question)
	period := rulePeriod(question)
	rest := text
	if period != "" {
		rest = strings.Replace(text, period, " ", 1)
	}
	reading, ok := readRuleWords(strings.Fields(nonWordRunes.ReplaceAllString(rest, " ")))
	if !ok {
		return Plan{}, false
	}
	return reading.plan(period, text)
}

// rulePeriod finds the period phrase in either date order: the rules only
// cut it out of the question, and Resolve reads it in the configured one.
func rulePeriod(question string) string {
	for _, order := range []timeline.DateOrder{timeline.DayFirst, timeline.MonthFirst} {
		if period, found := timeline.DayPhrase(question, ruleAnchor, order); found {
			return period
		}
	}
	return ""
}

// ruleReading is what the words of a question said.
type ruleReading struct {
	source    event.Source
	direction listing.Direction
	tasks     bool
	summary   bool
	listAll   bool
}

func readRuleWords(words []string) (ruleReading, bool) {
	var reading ruleReading
	for _, word := range words {
		if !reading.add(word) {
			return ruleReading{}, false
		}
	}
	return reading, true
}

// add records word; false when it is unknown or contradicts an earlier word
// ("commits e mensagens").
func (r *ruleReading) add(word string) bool {
	if source, ok := ruleSourceNouns[word]; ok {
		conflict := r.source != "" && r.source != source
		r.source = source
		return !conflict
	}
	if direction, ok := ruleDirectionWords[word]; ok {
		conflict := r.direction != listing.AnyDirection && r.direction != direction
		r.direction = direction
		return !conflict
	}
	return r.addCue(word)
}

func (r *ruleReading) addCue(word string) bool {
	switch {
	case ruleTaskNouns[word]:
		r.tasks = true
	case ruleSummaryWords[word]:
		r.summary = true
	case ruleListAllWords[word]:
		r.listAll = true
	default:
		return ruleGenericWords[word]
	}
	return true
}

// plan turns the reading into a Plan; false when it is ambiguous: a
// direction outside messages, a task question naming a source, or an
// "o que" that may ask for an answer or for the list.
func (r ruleReading) plan(period, text string) (Plan, bool) {
	if (r.direction != listing.AnyDirection && r.source != event.SourceTeams) || (r.tasks && r.source != "") {
		return Plan{}, false
	}
	plan := Plan{Period: period, Source: r.source, Criteria: listing.Criteria{Direction: r.direction}, ReadByRules: true}
	switch {
	case r.tasks:
		plan.Mode, plan.TaskStatus = ModeTasks, taskStatusIn(text)
	case r.summary || (r.source == "" && !r.listAll):
		plan.Mode = ModeAnswer
	case ruleAskingCues.MatchString(text):
		return Plan{}, false
	default:
		plan.Mode = ModeList
	}
	return plan, true
}

// taskStatusIn reads the status as guardTaskReport accepts it: "não
// finalizei" is unfinished, not done.
func taskStatusIn(text string) TaskStatus {
	switch {
	case inProgressCues.MatchString(text):
		return OnlyInProgress
	case doneCues.MatchString(text):
		return OnlyDone
	}
	return AnyStatus
}
