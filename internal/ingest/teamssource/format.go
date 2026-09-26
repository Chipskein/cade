package teamssource

import (
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/v8value"
)

// A Teams client update can rename the stores or fields read here. Without
// a check, ingestion would find no messages and report "0 novos" as if the
// cache had nothing new; formatTally turns that into an error.

// requiredMessageFields are the fields parseMessage relies on; a message
// lacking any of them is in a format this version does not know.
var requiredMessageFields = []string{"id", "conversationId", "messageType", "content"}

var arrivalTimeFields = []string{"originalArrivalTime", "clientArrivalTime"}

// formatTally counts what one collection saw.
type formatTally struct {
	records     int
	replyChains int
	messages    int
	recognized  int
}

func (t *formatTally) countMessage(value *v8value.Value) {
	t.messages++
	if isRecognized(value) {
		t.recognized++
	}
}

// isRecognized separates an unknown format from messages skipped on
// purpose (system, deleted, empty), which still have every field.
func isRecognized(value *v8value.Value) bool {
	for _, field := range requiredMessageFields {
		if value.Get(field) == nil {
			return false
		}
	}
	return value.Get(arrivalTimeFields[0]) != nil || value.Get(arrivalTimeFields[1]) != nil
}

// check fails when the IndexedDB has data but no message store, or message
// entries none of which has the expected fields. An empty IndexedDB passes:
// one of the two Teams origins can legitimately hold nothing.
func (t formatTally) check(dir string) error {
	if t.records > 0 && t.replyChains == 0 {
		return fmt.Errorf("formato do Teams não reconhecido em %q: %d registros e nenhum no store %q (bancos %q…); o cliente pode ter mudado — rode `cade teams-schema %s` e compare",
			dir, t.records, replyChainStore, replyChainDatabasePrefix, dir)
	}
	if t.messages > 0 && t.recognized == 0 {
		return fmt.Errorf("formato do Teams não reconhecido em %q: %d mensagens no cache e nenhuma com os campos %s e %s; o cliente pode ter mudado — rode `cade teams-schema %s` e compare",
			dir, t.messages, strings.Join(requiredMessageFields, ", "), strings.Join(arrivalTimeFields, " ou "), dir)
	}
	return nil
}
