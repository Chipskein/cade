package idbdiscovery

import (
	"context"
	"errors"
	"fmt"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/llm"
)

// Token budgets of the two answers: a store label and a path, then the
// filled target (a dozen fields of one or two paths).
const (
	maxStoreTokens  = 96
	maxFieldsTokens = 640
)

// ErrNoMessages means the proposed schema maps no message from the records
// it was discovered on; the Discovery still holds it, to review or edit.
var ErrNoMessages = errors.New("the proposed schema maps no message")

// Discoverer asks the local model for a schema.
type Discoverer struct {
	generator llm.StructuredGenerator
	limits    CatalogLimits
}

// NewDiscoverer builds a Discoverer.
//
//	discoverer := idbdiscovery.NewDiscoverer(generator, idbdiscovery.DefaultCatalogLimits)
//	found, err := discoverer.Discover(ctx, records, "whatsapp", "whatsapp")
func NewDiscoverer(generator llm.StructuredGenerator, limits CatalogLimits) Discoverer {
	return Discoverer{generator: generator, limits: limits}
}

// Discovery is a proposed schema and how it fared on the records it was
// discovered from.
type Discovery struct {
	Schema idbmap.Schema
	Tally  idbmap.Tally
	Events int
}

// Discover proposes a schema called name, whose events get source, from
// records. The model runs twice: to pick the store, then to fill fields.
func (d Discoverer) Discover(ctx context.Context, records []indexeddb.Record, name, source string) (Discovery, error) {
	catalog := BuildCatalog(records, d.limits)
	if len(catalog.Stores) == 0 {
		return Discovery{}, fmt.Errorf("none of the %d records decoded to a store with values, expected an IndexedDB the application has written", len(records))
	}
	store, each, err := d.chooseStore(ctx, catalog)
	if err != nil {
		return Discovery{}, err
	}
	schema, err := d.fillFields(ctx, catalog, store, each, schemaTarget{name: name, source: source})
	if err != nil {
		return Discovery{}, err
	}
	return try(schema, records)
}

func (d Discoverer) chooseStore(ctx context.Context, catalog Catalog) (StoreView, idbmap.Path, error) {
	raw, err := d.generator.GenerateStructured(ctx, storeMessages(catalog), maxStoreTokens, storeGrammar(catalog))
	if err != nil {
		return StoreView{}, "", fmt.Errorf("ask the model for the message store: %w", err)
	}
	var reply storeReply
	if err := decodeReply(raw, &reply); err != nil {
		return StoreView{}, "", err
	}
	store, err := catalog.Store(reply.Store)
	if reply.Each == nil {
		return store, "", err
	}
	return store, *reply.Each, err
}

func (d Discoverer) fillFields(ctx context.Context, catalog Catalog, store StoreView, each idbmap.Path, target schemaTarget) (idbmap.Schema, error) {
	grammar, err := fieldsGrammar(catalog, store, each)
	if err != nil {
		return idbmap.Schema{}, err
	}
	raw, err := d.generator.GenerateStructured(ctx, fieldsMessages(catalog, store, each), maxFieldsTokens, grammar)
	if err != nil {
		return idbmap.Schema{}, fmt.Errorf("ask the model for the message fields: %w", err)
	}
	var reply fieldsReply
	if err := decodeReply(raw, &reply); err != nil {
		return idbmap.Schema{}, err
	}
	return reply.schema(target, catalog, store, each)
}

// try applies the schema to the records it came from: a schema that maps
// nothing is returned with ErrNoMessages, for the user to review.
func try(schema idbmap.Schema, records []indexeddb.Record) (Discovery, error) {
	mapper, err := idbmap.NewMapper(schema)
	if err != nil {
		return Discovery{Schema: schema}, fmt.Errorf("the proposed schema is invalid: %w", err)
	}
	events, tally, err := mapper.Apply(records, "")
	found := Discovery{Schema: schema, Tally: tally, Events: len(events)}
	if err != nil {
		return found, err
	}
	if len(events) == 0 {
		return found, fmt.Errorf("%w (%d store records, %d items, %d kept)", ErrNoMessages, tally.StoreRecords, tally.Items, tally.Kept)
	}
	return found, nil
}
