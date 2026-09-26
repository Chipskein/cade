package rag

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/textnorm"
)

// Embeddings rank meaning well and exact tokens badly: a commit hash, a
// PR number or "PROJ-481" next to "PROJ-418". Keyword search (FTS5, BM25)
// covers those. Mode picks the combination.

// Mode selects how retrieval searches.
type Mode string

const (
	// ModeHybrid fuses vector and keyword rankings (default).
	ModeHybrid Mode = "hybrid"
	ModeVector Mode = "vector"
	// ModeLexical uses keywords only.
	ModeLexical Mode = "lexical"
)

// ParseMode reads retrieval.mode; empty means hybrid.
//
//	mode, err := rag.ParseMode(cfg.Retrieval.Mode)
func ParseMode(text string) (Mode, error) {
	switch mode := Mode(text); mode {
	case "", ModeHybrid:
		return ModeHybrid, nil
	case ModeVector, ModeLexical:
		return mode, nil
	}
	return "", fmt.Errorf("retrieval.mode %q, expected \"hybrid\", \"vector\" or \"lexical\"", text)
}

var (
	taskIDPattern   = regexp.MustCompile(`\b([A-Za-z]{2,10})-(\d+)\b`)
	hashPattern     = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	prNumberPattern = regexp.MustCompile(`(?i)\b(?:pr|pull request|merge request|mr)\s*#?\s*(\d+)\b`)
)

// identifierMatch turns explicit identifiers in the question into an FTS5
// expression: task and error codes as phrases (the tokenizer splits
// "PROJ-481" into "proj 481", which "PROJ-418" does not match), commit
// hashes as prefixes, PR numbers as the phrases their titles use.
//
//	identifierMatch("o que foi o commit e5f6a7b?") // `e5f6a7b*`
func identifierMatch(question string) string {
	var terms []string
	for _, match := range taskIDPattern.FindAllStringSubmatch(question, -1) {
		terms = append(terms, fmt.Sprintf(`"%s %s"`, strings.ToLower(match[1]), match[2]))
	}
	for _, hash := range hashPattern.FindAllString(strings.ToLower(question), -1) {
		if strings.ContainsAny(hash, "0123456789") && strings.ContainsAny(hash, "abcdef") {
			terms = append(terms, hash+"*")
		}
	}
	for _, match := range prNumberPattern.FindAllStringSubmatch(question, -1) {
		terms = append(terms, fmt.Sprintf(`"pull request %s" OR "merge request %s" OR "pr %s"`, match[1], match[1], match[1]))
	}
	return strings.Join(terms, " OR ")
}

// HasIdentifier reports whether the question names an identifier, which
// retrieval answers by keyword, bypassing the distance gates.
func HasIdentifier(question string) bool {
	return identifierMatch(question) != ""
}

// lexicalStopwords are question and function words, which match
// everything and rank nothing.
var lexicalStopwords = wordSet(`
o a os as um uma de do da dos das em no na nos nas por para pra pro com sem sobre que quem qual quais quando onde como
eu me meu minha meus minhas voce foi era fiz fez tem ter ja isso esse essa este esta ao aos e ou se mais
the a an of in on at to for with about what which who when where how did do does was were is are i my me it this that and or
ontem hoje yesterday today semana week mes month`)

// wordsMatch ORs the content words of text, folded, for BM25 ranking.
func wordsMatch(text string) string {
	var terms []string
	seen := map[string]bool{}
	for _, word := range strings.FieldsFunc(textnorm.Fold(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(word) >= 3 && !lexicalStopwords[word] && !seen[word] {
			seen[word] = true
			terms = append(terms, `"`+word+`"`)
		}
	}
	return strings.Join(terms, " OR ")
}

// searchLexical runs a keyword query with the question's filters and
// prepares the hits as the vector path does: one per event, scored by the
// vector distance of the chunk that matched, without chatter, removed
// files or repeats.
func (a *Answerer) searchLexical(ctx context.Context, embedding []float32, query queryplan.Query, match string) ([]storage.ScoredEvent, error) {
	lexical := storage.LexicalQuery{Match: match, Limit: a.settings.TopK * chatterHeadroom * 4, Source: query.Source}
	if query.Days != nil {
		lexical.From, lexical.To = query.Days.Start(), query.Days.End()
	}
	hits, err := a.store.SearchLexical(ctx, lexical)
	if err != nil || len(hits) == 0 {
		return nil, err
	}
	hits, err = a.withVectorDistances(ctx, embedding, bestChunkPerEvent(hits))
	if err != nil {
		return nil, err
	}
	hits = usableEvidence(hits, query)
	return hits[:min(len(hits), a.settings.TopK)], nil
}

// withVectorDistances gives keyword hits the distance of their matched
// chunk to the question, so the distance gates judge them like any hit.
func (a *Answerer) withVectorDistances(ctx context.Context, embedding []float32, hits []storage.ScoredEvent) ([]storage.ScoredEvent, error) {
	uids := make([]string, len(hits))
	for i, hit := range hits {
		uids[i] = hit.Event.UID
	}
	chunks, err := a.store.ChunksFor(ctx, uids)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		hits[i].Distance = matchedChunkDistance(embedding, hits[i], chunks[hits[i].Event.UID])
	}
	return hits, nil
}

func matchedChunkDistance(embedding []float32, hit storage.ScoredEvent, chunks []storage.Chunk) float64 {
	for _, chunk := range chunks {
		if chunk.Ordinal == hit.Chunk.Ordinal {
			return cosineDistance(embedding, chunk.Vector)
		}
	}
	return math.Inf(1)
}

// rrfK is the usual reciprocal rank fusion constant: ranks matter, but the
// top of one list does not drown the other.
const rrfK = 60

// fuseRankings merges rankings by reciprocal rank fusion, keeping each
// event's first occurrence (the vector one when both lists have it).
func fuseRankings(rankings ...[]storage.ScoredEvent) []storage.ScoredEvent {
	scores := map[string]float64{}
	var fused []storage.ScoredEvent
	for _, ranking := range rankings {
		for rank, hit := range ranking {
			if _, seen := scores[hit.Event.UID]; !seen {
				fused = append(fused, hit)
			}
			scores[hit.Event.UID] += 1 / float64(rrfK+rank+1)
		}
	}
	sort.SliceStable(fused, func(i, j int) bool { return scores[fused[i].Event.UID] > scores[fused[j].Event.UID] })
	return fused
}
