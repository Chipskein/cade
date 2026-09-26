package rag

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
)

// NotFoundMarker is the exact reply the model is told to give when the
// evidence does not answer the question; detecting it lets the CLI print a
// clear "not found" instead of whatever the model improvised (CA9).
const NotFoundMarker = "SEM_INFORMACAO"

// maxEvidenceChars bounds each event's text in the prompt so top_k events
// fit the generation context.
const maxEvidenceChars = 700

const evidenceTimeLayout = "2006-01-02 15:04"

var citationPattern = regexp.MustCompile(`\[(\d+)\]`)

const systemInstructions = `Você responde perguntas sobre o histórico de atividade do usuário (commits git, páginas visitadas no navegador, arquivos, mensagens do Teams).
Regras obrigatórias:
1. Use SOMENTE as informações dos eventos fornecidos. Não invente nada que não esteja neles.
2. Alguns eventos podem ser irrelevantes: ignore-os. Considere TODOS os eventos relevantes, de todas as fontes.
3. Cite cada evento usado pelo número entre colchetes, por exemplo [2], junto com a fonte e a data.
4. Perguntas como "o que eu fiz" ou "que páginas visitei" são respondidas listando os eventos fornecidos.
5. Somente se NENHUM evento tiver relação com a pergunta, responda exatamente: ` + NotFoundMarker + `
6. Responda com suas próprias palavras, no mesmo idioma da pergunta e de forma concisa. Não repita os cabeçalhos dos eventos.
7. Palavras como "ontem" dentro de um evento referem-se à data daquele evento, não à data atual.
8. Mensagens do Teams indicam se foram enviadas por você, recebidas por você ou publicadas num canal. Publicações em canal não são mensagens recebidas diretamente: só as use para perguntas sobre mensagens recebidas se nada mais responder, e diga que eram publicações em canal.`

func buildPrompt(question string, hits []storage.ScoredEvent, now time.Time) []llm.ChatMessage {
	user := fmt.Sprintf("Data e hora atual: %s\n\nEventos:\n%s\nPergunta: %s",
		now.Format(evidenceTimeLayout), formatEvidence(hits, now.Location()), question)
	return []llm.ChatMessage{
		{Role: llm.RoleSystem, Content: systemInstructions},
		{Role: llm.RoleUser, Content: user},
	}
}

// sourceLabels name sources in the evidence header. The header used to be
// "fonte=teams data=...", which the small model copied verbatim into its
// answer; a natural phrase reads fine even when copied.
var sourceLabels = map[event.Source]string{
	event.SourceGit: "Commit git", event.SourceBrowser: "Página visitada",
	event.SourceFile: "Arquivo", event.SourceTeams: "Mensagem do Teams",
}

func formatEvidence(hits []storage.ScoredEvent, location *time.Location) string {
	var builder strings.Builder
	for i, hit := range hits {
		fmt.Fprintf(&builder, "[%d] %s, %s\n%s\n\n", i+1, sourceLabel(hit.Event.Source),
			hit.Event.Timestamp.In(location).Format(evidenceTimeLayout), clip(hit.Event.Content, maxEvidenceChars))
	}
	return builder.String()
}

func sourceLabel(source event.Source) string {
	if label, known := sourceLabels[source]; known {
		return label
	}
	return string(source)
}

func clip(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func isNotFoundReply(reply string) bool {
	return strings.Contains(reply, NotFoundMarker) || strings.TrimSpace(reply) == ""
}

// citedIndexes returns the distinct 1-based evidence numbers the reply cites,
// ignoring numbers outside 1..count (the model occasionally invents some).
func citedIndexes(reply string, count int) []int {
	seen := map[int]bool{}
	for _, match := range citationPattern.FindAllStringSubmatch(reply, -1) {
		index, err := strconv.Atoi(match[1])
		if err == nil && index >= 1 && index <= count {
			seen[index] = true
		}
	}
	indexes := make([]int, 0, len(seen))
	for index := range seen {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}
