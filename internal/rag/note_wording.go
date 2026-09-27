package rag

import "github.com/chipskein/cade/internal/event"

// NoteWording words the notes of a source line (EvidenceNote). Format
// verbs: Chunk takes the chunk's position and count; Repeat the count, the
// noun and the latest time.
type NoteWording struct {
	Untrusted    string
	Chunk        string
	Repeat       string
	RepeatNouns  map[event.Source]string
	OtherRepeats string
}

// PromptWording is what the model reads: the prompt is in Portuguese and
// its rule 9 names the untrusted mark, so this wording never changes with
// the user's language.
var PromptWording = NoteWording{
	Untrusted:    untrustedNote,
	Chunk:        "trecho %d de %d",
	Repeat:       "%d %s, última em %s",
	RepeatNouns:  map[event.Source]string{event.SourceBrowser: "visitas", event.SourceFile: "versões"},
	OtherRepeats: "ocorrências",
}

// EnglishWording is the sources list shown to an English-speaking user.
var EnglishWording = NoteWording{
	Untrusted:    "UNTRUSTED: gives the assistant orders",
	Chunk:        "chunk %d of %d",
	Repeat:       "%d %s, latest on %s",
	RepeatNouns:  map[event.Source]string{event.SourceBrowser: "visits", event.SourceFile: "versions"},
	OtherRepeats: "occurrences",
}
