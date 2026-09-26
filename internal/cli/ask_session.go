package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/rag"
)

// askSession shows what `cade ask` is doing — loading models, searching,
// reading the prompt — on stderr, and streams the reply to stdout.
type askSession struct {
	env    commandEnv
	status statusLine
	stream answerStream
	// jsonOutput replaces the streamed reply and text rendering with one
	// JSON document at the end (`cade ask --json`).
	jsonOutput bool
}

func newAskSession(env commandEnv, jsonOutput bool) *askSession {
	streamTo := env.stdout
	if jsonOutput {
		streamTo = io.Discard
	}
	return &askSession{
		env:        env,
		status:     statusLine{out: env.stderr, interactive: env.toolkit.StderrIsTerminal},
		stream:     answerStream{out: streamTo},
		jsonOutput: jsonOutput,
	}
}

// writeReport prints the JSON report of query, completed by fill.
func (s *askSession) writeReport(query queryplan.Query, fill func(*askReport)) error {
	s.status.clear()
	report := newAskReport(query)
	fill(&report)
	return writeAskReport(s.env.stdout, report)
}

var stageMessages = map[rag.AnswerStage]string{
	rag.StageSearching:  "Buscando eventos…",
	rag.StageGenerating: "Gerando resposta…",
}

func (s *askSession) loadingModels() {
	s.status.show("Carregando modelos…")
}

func (s *askSession) observer() rag.AnswerObserver {
	return rag.AnswerObserver{
		StageStarted:   func(stage rag.AnswerStage) { s.status.show(stageMessages[stage]) },
		PeopleResolved: s.peopleResolved,
		Generation:     llm.GenerationProgress{PromptProcessed: s.promptProcessed, TokenGenerated: s.token},
	}
}

// promptProcessed shows a percentage only on a terminal; as log lines it
// would add one line per chunk.
func (s *askSession) promptProcessed(done, total int) {
	if s.status.interactive && total > 0 {
		s.status.show(fmt.Sprintf("Lendo contexto: %d%%", done*100/total))
	}
}

func (s *askSession) peopleResolved(matched, unknown []string) {
	s.status.clear()
	reportPeople(s.env.stderr, matched, unknown)
}

func (s *askSession) token(piece string) {
	s.status.clear()
	s.stream.write(piece)
}

// render prints what streaming did not: the sources, or the whole answer
// when nothing was streamed (e.g. "not found").
func (s *askSession) render(answer rag.Answer, location *time.Location) {
	s.status.clear()
	if s.stream.wroteAny() && answer.Found {
		fmt.Fprint(s.env.stdout, "\n\n")
		renderSources(s.env.stdout, answer, location)
		return
	}
	if s.stream.wroteAny() {
		fmt.Fprintln(s.env.stdout)
	}
	renderAnswer(s.env.stdout, answer, location)
}
