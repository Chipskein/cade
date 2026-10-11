package whatsappsource

import (
	"testing"

	"github.com/chipskein/cade/internal/msgstore"
)

func TestWebMessageID(t *testing.T) {
	cases := []struct {
		name     string
		message  msgstore.Message
		expected string
	}{
		{"one-to-one, phone only", msgstore.Message{KeyID: "K", ChatJID: "55@s.whatsapp.net"}, "false_55@c.us_K"},
		{"one-to-one with LID", msgstore.Message{KeyID: "K", FromMe: true, ChatJID: "55@s.whatsapp.net", ChatLID: "9@lid"}, "true_9@lid_K"},
		{"group, received", msgstore.Message{KeyID: "K", ChatJID: "1@g.us", SenderJID: "55@s.whatsapp.net", SenderLID: "9@lid"}, "false_1@g.us_K_9@lid"},
		{"group, sent", msgstore.Message{KeyID: "K", FromMe: true, ChatJID: "1@g.us", SenderJID: "77@s.whatsapp.net"}, "true_1@g.us_K"},
	}
	for _, c := range cases {
		if got := webMessageID(c.message); got != c.expected {
			t.Errorf("%s: expected %q, got %q", c.name, c.expected, got)
		}
	}
}

func TestWebMessageKeyReadsDirectionAndKey(t *testing.T) {
	key, ok := webMessageKey("true_1@g.us_K_9@lid")
	if !ok || key != (messageKey{fromMe: true, keyID: "K"}) {
		t.Fatalf("got %+v, %v", key, ok)
	}
	if _, ok := webMessageKey("sem-partes"); ok {
		t.Fatal("an id without parts has no key")
	}
}
