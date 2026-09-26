package llamacpp

import "unicode/utf8"

// utf8Streamer forwards token pieces as they arrive while accumulating the
// full reply. A token can end in the middle of a multi-byte character
// ("ã", emoji), so bytes are held back until they form valid UTF-8.
type utf8Streamer struct {
	emit    func(piece string)
	reply   []byte
	pending []byte
}

func (s *utf8Streamer) write(piece []byte) {
	s.reply = append(s.reply, piece...)
	s.pending = append(s.pending, piece...)
	if utf8.Valid(s.pending) {
		s.emit(string(s.pending))
		s.pending = s.pending[:0]
	}
}

// finish returns the whole reply; bytes still pending (a truncated
// character at maxTokens) are dropped from the stream but kept in the reply.
func (s *utf8Streamer) finish() string {
	return string(s.reply)
}
