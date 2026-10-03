// Package idbsource ingests an application's browser storage through a
// saved idbmap schema: no code of its own per application or storage, and
// no model at ingest time.
package idbsource

import (
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/webstore"
)

// Collector emits the messages one schema finds in one location.
type Collector struct {
	readers  webstore.Readers
	location string
	schema   idbmap.Schema
	mapper   idbmap.Mapper
}

// NewCollector compiles schema for location, which one of readers of the
// schema's kind reads.
//
//	collector, err := idbsource.NewCollector(readers, dir, schema)
func NewCollector(readers webstore.Readers, location string, schema idbmap.Schema) (*Collector, error) {
	mapper, err := idbmap.NewMapper(schema)
	if err != nil {
		return nil, err
	}
	return &Collector{readers: readers, location: location, schema: schema, mapper: mapper}, nil
}

// CollectEvents reads the location once, maps it and emits each event.
// Events emitted before a format error stay ingested; the error says the
// application may have changed its format.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	records, err := c.readers.ReadKind(c.schema.Records.Kind, c.location)
	if err != nil {
		return err
	}
	events, tally, err := c.mapper.Apply(records)
	if err != nil {
		return err
	}
	for _, mapped := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(mapped); err != nil {
			return err
		}
	}
	if err := tally.Check(c.schema); err != nil {
		return fmt.Errorf("%w; run `cade idb-check --update %s`", err, c.schema.Name)
	}
	return nil
}
