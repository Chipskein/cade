package cli

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// initPrompt asks `cade init`'s questions. At the end of the input every
// question takes its default, so init also runs unattended.
type initPrompt struct {
	answers  *bufio.Scanner
	out      io.Writer
	language Language
}

func newInitPrompt(in io.Reader, out io.Writer, language Language) *initPrompt {
	return &initPrompt{answers: bufio.NewScanner(in), out: out, language: language}
}

// line asks question and returns the trimmed answer, "" at end of input.
func (p *initPrompt) line(question string) string {
	fmt.Fprint(p.out, question)
	if !p.answers.Scan() {
		fmt.Fprintln(p.out)
		return ""
	}
	return strings.TrimSpace(p.answers.Text())
}

// lines collects one answer per line until an empty one.
func (p *initPrompt) lines(question string) []string {
	fmt.Fprintln(p.out, question)
	var answers []string
	for answer := p.line("> "); answer != ""; answer = p.line("> ") {
		answers = append(answers, answer)
	}
	return answers
}

// choose lists candidates under title and returns the ones the user
// includes; an empty answer includes all of them when includeAll.
func (p *initPrompt) choose(title string, candidates []string, includeAll bool) []string {
	fmt.Fprintf(p.out, "\n%s:\n", title)
	for i, candidate := range candidates {
		fmt.Fprintf(p.out, "  %d. %s\n", i+1, candidate)
	}
	for {
		indexes, understood := parseChoice(p.line(p.choiceQuestion(includeAll)), len(candidates), includeAll)
		if understood {
			return pickIndexes(candidates, indexes)
		}
		fmt.Fprintln(p.out, p.language.pick("Resposta não entendida.", "Answer not understood."))
	}
}

func (p *initPrompt) choiceQuestion(includeAll bool) string {
	if includeAll {
		return p.language.pick("Incluir quais? [T]odos, [n]enhum ou os números (ex.: 1 3) [todos]: ", "Include which? [A]ll, [n]one or the numbers (e.g. 1 3) [all]: ")
	}
	return p.language.pick("Incluir quais? [t]odos, [N]enhum ou os números (ex.: 1 3) [nenhum]: ", "Include which? [a]ll, [N]one or the numbers (e.g. 1 3) [none]: ")
}

// choiceWords are the answers meaning every candidate or none, in both
// languages.
var (
	allWords  = []string{"t", "todos", "a", "all", "*"}
	noneWords = []string{"n", "nenhum", "none", "-"}
)

// parseChoice reads an answer to choose as 0-based indexes among count
// candidates; false when it is neither a word above nor numbers in range.
func parseChoice(answer string, count int, includeAll bool) ([]int, bool) {
	answer = strings.ToLower(answer)
	switch {
	case answer == "" && includeAll, slices.Contains(allWords, answer):
		return allIndexes(count), true
	case answer == "", slices.Contains(noneWords, answer):
		return nil, true
	}
	return parseNumbers(answer, count)
}

func parseNumbers(answer string, count int) ([]int, bool) {
	var indexes []int
	for _, field := range strings.FieldsFunc(answer, func(r rune) bool { return r == ' ' || r == ',' }) {
		number, err := strconv.Atoi(field)
		if err != nil || number < 1 || number > count {
			return nil, false
		}
		if !slices.Contains(indexes, number-1) {
			indexes = append(indexes, number-1)
		}
	}
	return indexes, true
}

func allIndexes(count int) []int {
	indexes := make([]int, count)
	for i := range indexes {
		indexes[i] = i
	}
	return indexes
}

func pickIndexes(candidates []string, indexes []int) []string {
	picked := make([]string, 0, len(indexes))
	for _, index := range indexes {
		picked = append(picked, candidates[index])
	}
	return picked
}
