package event

import "strings"

// A Teams event's content is built from its Message metadata alone, so the
// content (and its embedding) can be rebuilt from the database when this
// layout changes, without the Teams cache, which expires.

// Content renders m as stored and embedded: "Sender: text", then the
// conversation and direction lines. Those lines let search and the model
// tell a direct message from a channel post.
//
//	content := event.Message{Sender: "Ana", Text: "oi", Kind: event.KindChat}.Content()
func (m Message) Content() string {
	return m.Sender + ": " + m.Text + "\n" + m.conversationLine() + "\n" + m.directionLine()
}

func (m Message) conversationLine() string {
	if m.Conversation == "" {
		return "Conversa: " + string(m.Kind)
	}
	return "Conversa: " + string(m.Kind) + " " + m.Conversation
}

func (m Message) directionLine() string {
	switch {
	case m.SentByMe:
		return "Enviada por você"
	case m.Kind == KindChannel:
		return "Publicada no canal (não enviada diretamente a você)"
	}
	return "Recebida por você"
}

// MessageText recovers the message's own text from stored content, for
// events ingested before the text was kept in metadata. It succeeds only
// when rendering m with that text reproduces content exactly, so content
// in an older layout is never misread.
//
//	text, ok := event.MessageText(stored.Content, stored.Message())
func MessageText(content string, m Message) (string, bool) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return "", false
	}
	text, found := strings.CutPrefix(strings.Join(lines[:len(lines)-2], "\n"), m.Sender+": ")
	m.Text = text
	if !found || m.Content() != content {
		return "", false
	}
	return text, true
}
