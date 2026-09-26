package event

import (
	"strings"
	"testing"
)

func TestContentLines(t *testing.T) {
	chat := Message{Sender: "Ana", Text: "oi", Kind: KindChat}
	meeting := Message{Sender: "Ana", Text: "oi", Kind: KindMeeting, Conversation: "Daily", SentByMe: true}
	channel := Message{Sender: "Ana", Text: "oi", Kind: KindChannel}
	if chat.Content() != "Ana: oi\nConversa: chat\nRecebida por você" || meeting.Content() != "Ana: oi\nConversa: reunião Daily\nEnviada por você" {
		t.Fatalf("unexpected content %q / %q", chat.Content(), meeting.Content())
	}
	if !strings.HasSuffix(channel.Content(), "Publicada no canal (não enviada diretamente a você)") {
		t.Fatalf("unexpected channel content %q", channel.Content())
	}
}

func TestMessageTextRecoversMultilineText(t *testing.T) {
	m := Message{Sender: "Ana Souza", Kind: KindChat, Conversation: "Ana Souza, Eu", Text: "linha 1\nlinha 2: com dois pontos"}
	stored := m.Content()
	m.Text = ""
	if text, ok := MessageText(stored, m); !ok || text != "linha 1\nlinha 2: com dois pontos" {
		t.Fatalf("expected the multiline text back, got %q %v", text, ok)
	}
}

// Content in another layout must not be misread as a message's text.
func TestMessageTextRejectsOtherLayouts(t *testing.T) {
	m := Message{Sender: "Ana", Kind: KindChat}
	for _, content := range []string{"Ana: oi", "Ana: oi\nRecebida por você", "Rui: oi\nConversa: chat\nRecebida por você", "Ana: oi\nConversa: canal\nRecebida por você"} {
		if text, ok := MessageText(content, m); ok {
			t.Errorf("expected %q rejected, got %q", content, text)
		}
	}
}
