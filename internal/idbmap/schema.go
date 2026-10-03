// Package idbmap is the declarative mapping from decoded browser-storage
// records (webstore: IndexedDB first) to cade messages. A Schema is generated once per application (by the
// local model, see idbdiscovery) and kept outside the code; applying it is
// deterministic and never runs a model.
package idbmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/webstore"
)

// FormatVersion is the schema file format this code writes; it also reads
// version 1 (see upgrade).
const FormatVersion = 2

// Target names the cade entity a schema fills, with its version: a change
// to the entity means migrating saved schemas, not regenerating them.
type Target string

const TargetMessage Target = "message/1"

// Field is one slot of the target entity.
type Field string

const (
	FieldMessageID      Field = "message_id"
	FieldConversationID Field = "conversation_id"
	FieldConversation   Field = "conversation"
	FieldSender         Field = "sender"
	FieldSenderID       Field = "sender_id"
	FieldText           Field = "text"
	FieldSentAt         Field = "sent_at"
	FieldSentByMe       Field = "sent_by_me"
	FieldRevision       Field = "revision"
)

// fieldShape says how a field's value is read.
type fieldShape int

const (
	shapeText fieldShape = iota
	shapeTime
	shapeFlag
)

var fieldShapes = map[Field]fieldShape{
	FieldMessageID: shapeText, FieldConversationID: shapeText, FieldConversation: shapeText,
	FieldSender: shapeText, FieldSenderID: shapeText, FieldText: shapeText, FieldRevision: shapeText,
	FieldSentAt: shapeTime, FieldSentByMe: shapeFlag,
}

// requiredFields are what a message needs to exist: an identity (the UID
// is built from both ids) and a time. Text is optional because some apps
// encrypt the body and only metadata can be indexed.
var requiredFields = []Field{FieldMessageID, FieldConversationID, FieldSentAt}

// Schema maps one application's browser storage to messages.
//
//	schema, err := idbmap.Parse(raw)
type Schema struct {
	Version  int                 `json:"version"`
	Target   Target              `json:"target"`
	Name     string              `json:"name"`
	Source   string              `json:"source"`
	Revision int                 `json:"revision"`
	Records  RecordSelector      `json:"records"`
	Require  []Condition         `json:"require,omitempty"`
	Fields   map[Field]FieldRule `json:"fields"`
	// Fingerprint is the shape of what the schema reads when it was made;
	// MeasureDrift compares it with the application's storage now.
	Fingerprint []PathShape `json:"fingerprint,omitempty"`
}

// RecordSelector picks the records that hold messages: those of the
// storage of Kind at Location, and, when Each is set, every value Each
// reaches inside one record (a chat record holding a map of messages).
// Lookups read the same storage.
type RecordSelector struct {
	Kind webstore.Kind `json:"kind"`
	Location
	Each Path `json:"each,omitempty"`
}

// Condition keeps an item only when the string at Path is in In, is not in
// NotIn, and the value there is not of kind KindNot.
type Condition struct {
	Path    Path     `json:"path"`
	In      []string `json:"in,omitempty"`
	NotIn   []string `json:"not_in,omitempty"`
	KindNot string   `json:"kind_not,omitempty"`
}

// FieldRule fills a field from the first of Paths that has a value, then
// from Lookup, then Default. A Required field left empty drops the item,
// which conditions on raw values cannot do (an HTML body that is only
// tags becomes empty text after html_text).
type FieldRule struct {
	Paths     []Path    `json:"paths,omitempty"`
	Split     *Split    `json:"split,omitempty"`
	Transform Transform `json:"transform,omitempty"`
	Lookup    *Lookup   `json:"lookup,omitempty"`
	Default   string    `json:"default,omitempty"`
	Required  bool      `json:"required,omitempty"`
}

// Split keeps one segment of a composite value, before the transform:
// WhatsApp's message id "false_<chat>_<id>" split on "_" at 0 says
// whether the user sent it, at 1 which chat it belongs to.
type Split struct {
	Separator string `json:"separator"`
	Index     int    `json:"index"`
}

// Lookup reads the first of Values with a value from the record at another
// location whose Match equals the item's key: the text at KeyPath, or the
// value already computed for KeyField (a chat id cut out of a composite
// key with Split). A sender id becomes a contact name this way.
type Lookup struct {
	Location
	KeyPath  Path   `json:"key_path,omitempty"`
	KeyField Field  `json:"key_field,omitempty"`
	Match    Path   `json:"match"`
	Values   []Path `json:"values"`
}

// namePattern keeps schema names and sources usable as file names and
// source names on the command line.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Parse reads a schema file and validates it. Unknown keys are rejected
// so a typo in a hand-edited schema fails loudly instead of being ignored.
func Parse(raw []byte) (Schema, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var schema Schema
	if err := decoder.Decode(&schema); err != nil {
		return Schema{}, fmt.Errorf("parse schema: %w", err)
	}
	if err := schema.upgrade(); err != nil {
		return Schema{}, fmt.Errorf("schema %q: %w", schema.Name, err)
	}
	return schema, schema.Validate()
}

// Validate reports the first problem that would make the schema unusable.
func (s Schema) Validate() error {
	checks := []func() error{s.validateHeader, s.validateRecords, s.validateConditions, s.validateFields}
	for _, check := range checks {
		if err := check(); err != nil {
			return fmt.Errorf("schema %q: %w", s.Name, err)
		}
	}
	return nil
}

func (s Schema) validateHeader() error {
	if s.Version != FormatVersion {
		return fmt.Errorf("version %d, expected %d (or %d, read as %d)", s.Version, FormatVersion, indexedDBOnlyVersion, FormatVersion)
	}
	if s.Target != TargetMessage {
		return fmt.Errorf("target %q, expected %q", s.Target, TargetMessage)
	}
	if !namePattern.MatchString(s.Name) || !namePattern.MatchString(s.Source) {
		return fmt.Errorf("name %q / source %q, expected lowercase letters, digits and dashes", s.Name, s.Source)
	}
	if s.Revision < 0 {
		return fmt.Errorf("revision %d, expected 0 or more", s.Revision)
	}
	return nil
}

func (s Schema) validateRecords() error {
	if err := s.Records.Kind.Validate(); err != nil {
		return fmt.Errorf("records.kind: %w", err)
	}
	if s.Records.Container == "" {
		return fmt.Errorf("records.container is empty, expected the container holding the messages (the object store, in IndexedDB)")
	}
	return validatePaths(s.Records.Each)
}

func (s Schema) validateConditions() error {
	for _, condition := range s.Require {
		if err := validatePaths(condition.Path); err != nil {
			return err
		}
		if _, err := parseKind(condition.KindNot); err != nil {
			return err
		}
	}
	return nil
}

func (s Schema) validateFields() error {
	for _, field := range requiredFields {
		if _, ok := s.Fields[field]; !ok {
			return fmt.Errorf("field %q is missing, required: %s", field, joinFields(requiredFields))
		}
	}
	for field, rule := range s.Fields {
		if err := validateRule(field, rule, s.Fields); err != nil {
			return fmt.Errorf("field %q: %w", field, err)
		}
	}
	return nil
}

func validateRule(field Field, rule FieldRule, fields map[Field]FieldRule) error {
	shape, known := fieldShapes[field]
	if !known {
		return fmt.Errorf("unknown field, expected one of %s", joinFields(sortedFields()))
	}
	if len(rule.Paths) == 0 && rule.Lookup == nil && rule.Default == "" {
		return fmt.Errorf("no paths, lookup or default, expected at least one")
	}
	if err := validateTransform(shape, rule.Transform); err != nil {
		return err
	}
	if err := validatePaths(rule.Paths...); err != nil {
		return err
	}
	if rule.Split != nil && (rule.Split.Separator == "" || rule.Split.Index < 0) {
		return fmt.Errorf("split %+v, expected a separator and an index of 0 or more", *rule.Split)
	}
	return validateLookup(field, rule.Lookup, fields)
}

func validateLookup(field Field, lookup *Lookup, fields map[Field]FieldRule) error {
	if lookup == nil {
		return nil
	}
	if lookup.Container == "" || lookup.Match == "" || len(lookup.Values) == 0 || (lookup.KeyPath == "") == (lookup.KeyField == "") {
		return fmt.Errorf("lookup %+v, expected store, match, values and one of key_path or key_field", *lookup)
	}
	if err := validateKeyField(field, lookup.KeyField, fields); err != nil {
		return err
	}
	return validatePaths(append([]Path{lookup.KeyPath, lookup.Match}, lookup.Values...)...)
}

// validateKeyField allows one level of lookup by field: the key field must
// exist and not be looked up by field itself, so values never loop.
func validateKeyField(field, keyField Field, fields map[Field]FieldRule) error {
	if keyField == "" {
		return nil
	}
	keyRule, exists := fields[keyField]
	if !exists || keyField == field || fieldShapes[keyField] != shapeText {
		return fmt.Errorf("lookup key_field %q, expected another text field of the schema", keyField)
	}
	if keyRule.Lookup != nil && keyRule.Lookup.KeyField != "" {
		return fmt.Errorf("lookup key_field %q is itself looked up by field, expected a field read from paths", keyField)
	}
	return nil
}

func validatePaths(paths ...Path) error {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := CompilePath(path); err != nil {
			return err
		}
	}
	return nil
}

func sortedFields() []Field {
	fields := make([]Field, 0, len(fieldShapes))
	for field := range fieldShapes {
		fields = append(fields, field)
	}
	slices.Sort(fields)
	return fields
}

func joinFields(fields []Field) string {
	names := make([]string, len(fields))
	for i, field := range fields {
		names[i] = string(field)
	}
	return strings.Join(names, ", ")
}
