package idbmap

import (
	"fmt"
	"strconv"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
)

// Mapper applies one validated schema, its paths compiled once.
type Mapper struct {
	schema     Schema
	each       *CompiledPath
	conditions []compiledCondition
	rules      map[Field]compiledRule
}

type compiledRule struct {
	rule  FieldRule
	paths []CompiledPath
}

// NewMapper validates and compiles schema.
//
//	mapper, err := idbmap.NewMapper(schema)
//	events, tally := mapper.Apply(records, "https+++web.whatsapp.com")
func NewMapper(schema Schema) (Mapper, error) {
	if err := schema.Validate(); err != nil {
		return Mapper{}, err
	}
	mapper := Mapper{schema: schema, rules: map[Field]compiledRule{}}
	var err error
	if mapper.each, err = compileEach(schema.Records.Each); err != nil {
		return Mapper{}, err
	}
	if mapper.conditions, err = compileConditions(schema.Require); err != nil {
		return Mapper{}, err
	}
	for field, rule := range schema.Fields {
		paths, err := compilePaths(rule.Paths...)
		if err != nil {
			return Mapper{}, err
		}
		mapper.rules[field] = compiledRule{rule: rule, paths: paths}
	}
	return mapper, nil
}

func compileEach(each Path) (*CompiledPath, error) {
	if each == "" {
		return nil, nil
	}
	compiled, err := CompilePath(each)
	return &compiled, err
}

// Tally counts what Apply saw, so a change in the application's format is
// reported instead of silently producing nothing.
type Tally struct {
	Records      int // decoded or not, in every store
	StoreRecords int // decoded records of the schema's store
	Items        int // items selected inside them
	Kept         int // items that passed the conditions
	Mapped       int // kept items that became events
}

// Check reports a format the schema no longer matches: records exist but
// none is in the store, or items pass the conditions but none maps. An
// empty database is fine.
func (t Tally) Check(schema Schema) error {
	if t.Records > 0 && t.StoreRecords == 0 {
		return fmt.Errorf("schema %q: no decoded record in store %q of a database starting with %q among %d records; the application may have changed its format", schema.Name, schema.Records.Container, schema.Records.NamespacePrefix, t.Records)
	}
	if t.Kept > 0 && t.Mapped == 0 {
		return fmt.Errorf("schema %q: none of %d items has %s; the application may have changed its format", schema.Name, t.Kept, joinFields(requiredFields))
	}
	return nil
}

// Apply maps records to events. origin names where the records came from
// (kept in metadata, not in the UID).
func (m Mapper) Apply(records []indexeddb.Record, origin string) ([]event.Event, Tally, error) {
	lookups, err := m.buildLookups(records)
	if err != nil {
		return nil, Tally{}, err
	}
	tally := Tally{Records: len(records)}
	var events []event.Event
	for _, record := range records {
		if !m.schema.Records.selects(record) {
			continue
		}
		tally.StoreRecords++
		events = append(events, m.mapRecord(record.Value, lookups, origin, &tally)...)
	}
	return events, tally, nil
}

func (m Mapper) buildLookups(records []indexeddb.Record) (map[Field]lookupIndex, error) {
	lookups := map[Field]lookupIndex{}
	for field, rule := range m.rules {
		if rule.rule.Lookup == nil {
			continue
		}
		index, err := buildLookup(*rule.rule.Lookup, records)
		if err != nil {
			return nil, err
		}
		lookups[field] = index
	}
	return lookups, nil
}

func (m Mapper) mapRecord(root *v8value.Value, lookups map[Field]lookupIndex, origin string, tally *Tally) []event.Event {
	var events []event.Event
	for _, item := range m.items(root) {
		tally.Items++
		if !keepsAll(m.conditions, item) {
			continue
		}
		tally.Kept++
		if mapped, ok := m.mapItem(item, lookups, origin); ok {
			tally.Mapped++
			events = append(events, mapped)
		}
	}
	return events
}

func (m Mapper) items(root *v8value.Value) []*v8value.Value {
	if m.each == nil {
		return []*v8value.Value{root}
	}
	return m.each.Resolve(root)
}

// mapItem builds the event, keyed like the Teams collector on conversation
// and message id so a re-ingest or a second origin stays one event.
func (m Mapper) mapItem(item *v8value.Value, lookups map[Field]lookupIndex, origin string) (event.Event, bool) {
	message := m.message(item, lookups, origin)
	sentAt, hasTime := m.time(item)
	if message.MessageID == "" || message.ConversationID == "" || !hasTime || m.missesRequired(item, lookups) {
		return event.Event{}, false
	}
	source := event.Source(m.schema.Source)
	metadata := message.Metadata()
	metadata[event.SchemaKey], metadata[event.SchemaRevisionKey] = m.schema.Name, strconv.Itoa(m.schema.Revision)
	return event.Event{
		UID:       event.StableID(source, message.ConversationID, message.MessageID),
		Timestamp: sentAt,
		Source:    source,
		Content:   message.Content(),
		Metadata:  metadata,
	}, true
}

func (m Mapper) message(item *v8value.Value, lookups map[Field]lookupIndex, origin string) event.Message {
	text := func(field Field) string { return m.text(field, item, lookups) }
	return event.Message{
		MessageID: text(FieldMessageID), ConversationID: text(FieldConversationID), Conversation: text(FieldConversation),
		Kind: event.KindOther, Sender: text(FieldSender), SenderMRI: text(FieldSenderID), Text: text(FieldText),
		Revision: text(FieldRevision), SentByMe: m.flag(FieldSentByMe, item), Origin: origin,
	}
}

// text fills a text field: the first path with a value, then the lookup,
// then the default.
func (m Mapper) text(field Field, item *v8value.Value, lookups map[Field]lookupIndex) string {
	rule := m.rules[field]
	for _, path := range rule.paths {
		if text := ruleText(path.First(item), rule.rule); text != "" {
			return text
		}
	}
	if found := m.lookedUp(field, item, lookups); found != "" {
		return found
	}
	return rule.rule.Default
}

// lookedUp finds field's value in its lookup store, keyed by a path of the
// item or by another field's value (validated not to loop).
func (m Mapper) lookedUp(field Field, item *v8value.Value, lookups map[Field]lookupIndex) string {
	index, ok := lookups[field]
	if !ok {
		return ""
	}
	if index.keyField == "" {
		return index.findByPath(item)
	}
	return index.values[m.text(index.keyField, item, lookups)]
}

// missesRequired reports a required text field left empty.
func (m Mapper) missesRequired(item *v8value.Value, lookups map[Field]lookupIndex) bool {
	for field, rule := range m.rules {
		if rule.rule.Required && fieldShapes[field] == shapeText && m.text(field, item, lookups) == "" {
			return true
		}
	}
	return false
}

func (m Mapper) time(item *v8value.Value) (time.Time, bool) {
	rule := m.rules[FieldSentAt]
	for _, path := range rule.paths {
		if sentAt, ok := readTime(path.First(item), rule.rule.Transform); ok {
			return sentAt, true
		}
	}
	return time.Time{}, false
}

func (m Mapper) flag(field Field, item *v8value.Value) bool {
	rule := m.rules[field]
	for _, path := range rule.paths {
		if ruleFlag(path.First(item), rule.rule) {
			return true
		}
	}
	return false
}
