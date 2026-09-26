package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/chipskein/cade/internal/rag"
)

type streamState int

const (
	streamUndecided streamState = iota
	streamPassing
	streamSuppressed
)

// answerStream prints the reply as the model produces it. The model answers
// rag.NotFoundMarker when it has no answer, and that marker must never be
// shown, so text is held back only while it could still turn into it.
type answerStream struct {
	out   io.Writer
	held  string
	state streamState
}

func (s *answerStream) write(piece string) {
	switch s.state {
	case streamPassing:
		fmt.Fprint(s.out, piece)
	case streamUndecided:
		s.held += piece
		s.decide()
	}
}

func (s *answerStream) decide() {
	trimmed := strings.TrimLeft(s.held, " \t\n")
	if strings.HasPrefix(trimmed, rag.NotFoundMarker) {
		s.state = streamSuppressed
		return
	}
	if strings.HasPrefix(rag.NotFoundMarker, trimmed) {
		return
	}
	s.state = streamPassing
	fmt.Fprint(s.out, trimmed)
}

// wroteAny reports whether any reply text reached the output.
func (s *answerStream) wroteAny() bool {
	return s.state == streamPassing
}
