package cli

import (
	"fmt"
	"io"
)

// clearLine returns the cursor to column 0 and erases the line.
const clearLine = "\r\x1b[K"

// statusLine shows transient status on stderr. On a terminal each message
// replaces the previous one in place; otherwise each is a plain log line.
type statusLine struct {
	out         io.Writer
	interactive bool
	visible     bool
}

func (s *statusLine) show(text string) {
	if !s.interactive {
		fmt.Fprintln(s.out, text)
		return
	}
	fmt.Fprint(s.out, clearLine+text)
	s.visible = true
}

// clear erases the in-place line so regular output starts on a clean line.
func (s *statusLine) clear() {
	if s.visible {
		fmt.Fprint(s.out, clearLine)
		s.visible = false
	}
}
