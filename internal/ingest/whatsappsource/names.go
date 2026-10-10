package whatsappsource

import (
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/msgstore"
)

// unknownSender is the name the WhatsApp schema gives a sender missing
// from the contacts (testdata/idb-schemas/whatsapp.json), so both sources
// read alike. msgstore.db has no contact names: they are in wa.db.
const unknownSender = "desconhecido"

// contactNames are the names WhatsApp Web's events already carry, by id:
// the senders' (from the address book) and the conversations'.
type contactNames struct {
	senders       map[string]string
	conversations map[string]string
	me            string
}

func newContactNames(stored []event.Event) contactNames {
	names := contactNames{senders: map[string]string{}, conversations: map[string]string{}, me: unknownSender}
	for _, ev := range stored {
		names.learn(ev.Message())
	}
	return names
}

func (n *contactNames) learn(message event.Message) {
	if message.Conversation != "" {
		n.conversations[message.ConversationID] = message.Conversation
	}
	if message.Sender == "" || message.Sender == unknownSender {
		return
	}
	if message.SentByMe {
		n.me = message.Sender
		return
	}
	n.senders[message.SenderMRI] = message.Sender
}

// sender names who wrote message.
func (n contactNames) sender(message msgstore.Message) string {
	if message.FromMe {
		return n.me
	}
	if name, found := n.senders[senderID(message)]; found {
		return name
	}
	return unknownSender
}

// conversation names the chat: a group's subject, else the Web's name.
func (n contactNames) conversation(message msgstore.Message) string {
	if message.Subject != "" {
		return message.Subject
	}
	return n.conversations[chatID(message)]
}
