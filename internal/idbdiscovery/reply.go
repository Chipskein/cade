package idbdiscovery

import (
	"encoding/json"
	"fmt"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/idbschema"
)

// storeReply is the first answer: the store and its message items.
type storeReply struct {
	Store string       `json:"store"`
	Each  *idbmap.Path `json:"each"`
}

// fieldsReply is the second answer, the target filled from item paths.
type fieldsReply struct {
	MessageID      []idbmap.Path `json:"message_id"`
	ConversationID []idbmap.Path `json:"conversation_id"`
	SentAt         []idbmap.Path `json:"sent_at"`
	TimeFormat     string        `json:"time_format"`
	Sender         []idbmap.Path `json:"sender"`
	SenderID       []idbmap.Path `json:"sender_id"`
	Conversation   []idbmap.Path `json:"conversation"`
	Text           []idbmap.Path `json:"text"`
	TextFormat     string        `json:"text_format"`
	SentByMe       []idbmap.Path `json:"sent_by_me"`
	Keep           *keepReply    `json:"keep"`
	SenderLookup   *lookupReply  `json:"sender_lookup"`
}

type keepReply struct {
	Path idbmap.Path `json:"path"`
	In   []string    `json:"in"`
}

type lookupReply struct {
	Store string      `json:"store"`
	Key   idbmap.Path `json:"key"`
	Match idbmap.Path `json:"match"`
	Value idbmap.Path `json:"value"`
}

// unknownSender is the sender of a message whose sender has no name, as
// the Teams collector writes it.
const unknownSender = "desconhecido"

var timeTransforms = map[string]idbmap.Transform{
	formatUnixMS: idbmap.TransformUnixMS, formatUnixS: idbmap.TransformUnixS,
	formatISO8601: idbmap.TransformISO8601, formatDate: idbmap.TransformNone,
}

var textTransforms = map[string]idbmap.Transform{formatPlain: idbmap.TransformTrim, formatHTML: idbmap.TransformHTMLText}

func decodeReply(raw string, reply any) error {
	if err := json.Unmarshal([]byte(raw), reply); err != nil {
		return fmt.Errorf("model reply %q, expected the JSON the grammar allows: %w", raw, err)
	}
	return nil
}

// schemaTarget is where the schema is saved and what it is called.
type schemaTarget struct {
	name, source string
}

// schema builds the idbmap schema the replies describe; it is validated
// by the caller.
func (r fieldsReply) schema(target schemaTarget, catalog Catalog, store StoreView, each idbmap.Path) (idbmap.Schema, error) {
	schema := idbmap.Schema{
		Version: idbmap.FormatVersion, Target: idbmap.TargetMessage, Name: target.name, Source: target.source, Revision: 1,
		Records: idbmap.RecordSelector{Location: store.location(), Each: each},
		Fields:  r.fields(),
	}
	if r.Keep != nil {
		schema.Require = []idbmap.Condition{{Path: r.Keep.Path, In: r.Keep.In}}
	}
	if r.SenderLookup == nil {
		return schema, nil
	}
	lookup, err := r.SenderLookup.lookup(catalog)
	sender := schema.Fields[idbmap.FieldSender]
	sender.Lookup = lookup
	schema.Fields[idbmap.FieldSender] = sender
	return schema, err
}

func (r fieldsReply) fields() map[idbmap.Field]idbmap.FieldRule {
	textTransform := textTransforms[r.TextFormat]
	candidates := map[idbmap.Field]idbmap.FieldRule{
		idbmap.FieldMessageID:      {Paths: r.MessageID},
		idbmap.FieldConversationID: {Paths: r.ConversationID},
		idbmap.FieldSentAt:         {Paths: r.SentAt, Transform: timeTransforms[r.TimeFormat]},
		idbmap.FieldSender:         {Paths: r.Sender, Transform: idbmap.TransformTrim, Default: unknownSender},
		idbmap.FieldSenderID:       {Paths: r.SenderID},
		idbmap.FieldConversation:   {Paths: r.Conversation, Transform: idbmap.TransformTrim},
		idbmap.FieldText:           {Paths: r.Text, Transform: textTransform},
		idbmap.FieldSentByMe:       {Paths: r.SentByMe},
	}
	fields := map[idbmap.Field]idbmap.FieldRule{}
	for field, rule := range candidates {
		if len(rule.Paths) > 0 || rule.Default != "" {
			fields[field] = rule
		}
	}
	return fields
}

func (l lookupReply) lookup(catalog Catalog) (*idbmap.Lookup, error) {
	store, err := catalog.Store(l.Store)
	if err != nil {
		return nil, fmt.Errorf("sender lookup: %w", err)
	}
	return &idbmap.Lookup{Location: store.location(), KeyPath: l.Key, Match: l.Match, Values: []idbmap.Path{l.Value}}, nil
}

// location is where a schema finds the store: its database cut to the
// stable prefix, since the rest often names the user.
func (s StoreView) location() idbmap.Location {
	return idbmap.Location{NamespacePrefix: idbschema.StablePrefix(s.Database), Container: s.Store}
}
