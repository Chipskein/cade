package idbdiscovery

import (
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/idbmap"
)

// regenerationNote tells the model the application changed: the mapping
// it had (paths only, never values) and what drifted. Keeping the same
// identity fields keeps the UIDs of what is already indexed.
func regenerationNote(current idbmap.Schema, drift idbmap.Drift) string {
	lines := []string{
		"O aplicativo mudou o formato desde o schema anterior. Mantenha os campos que ainda existem, principalmente message_id e conversation_id, e troque só o que sumiu ou mudou.",
		fmt.Sprintf("Schema anterior: store %q, each %q.", current.Records.Container, current.Records.Each),
	}
	for _, field := range sortedFieldNames(current) {
		lines = append(lines, fmt.Sprintf("- %s: %s", field, joinPaths(current.Fields[field].Paths)))
	}
	if paths := shapePaths(drift.Missing, false); paths != "" {
		lines = append(lines, "Caminhos que sumiram: "+paths+".")
	}
	if paths := shapePaths(drift.Changed, true); paths != "" {
		lines = append(lines, "Caminhos que mudaram de tipo: "+paths+".")
	}
	return strings.Join(lines, "\n")
}

func sortedFieldNames(schema idbmap.Schema) []idbmap.Field {
	var fields []idbmap.Field
	for _, field := range []idbmap.Field{idbmap.FieldMessageID, idbmap.FieldConversationID, idbmap.FieldSentAt, idbmap.FieldSender,
		idbmap.FieldSenderID, idbmap.FieldConversation, idbmap.FieldText, idbmap.FieldSentByMe, idbmap.FieldRevision} {
		if rule, exists := schema.Fields[field]; exists && len(rule.Paths) > 0 {
			fields = append(fields, field)
		}
	}
	return fields
}

func joinPaths(paths []idbmap.Path) string {
	texts := make([]string, len(paths))
	for i, path := range paths {
		texts[i] = string(path)
	}
	return strings.Join(texts, ", ")
}

// shapePaths lists drifted paths, with their kinds now when they changed.
func shapePaths(shapes []idbmap.PathShape, withKinds bool) string {
	texts := make([]string, len(shapes))
	for i, shape := range shapes {
		texts[i] = string(shape.Path)
		if withKinds && len(shape.Kinds) > 0 {
			texts[i] += " (" + strings.Join(shape.Kinds, "|") + ")"
		}
	}
	return strings.Join(texts, ", ")
}
