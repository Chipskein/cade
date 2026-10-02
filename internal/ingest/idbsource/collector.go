// Package idbsource ingests an application's IndexedDB through a saved
// idbmap schema: no code of its own per application and no model at
// ingest time.
package idbsource

import (
	"context"

	"github.com/chipskein/cade/internal/idbmap"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/ingest"
)

// ReadIndexedDB reads every record of an IndexedDB directory, Chromium or
// Firefox; injected so tests need no browser profile.
type ReadIndexedDB func(dir string) ([]indexeddb.Record, error)

// Collector emits the messages one schema finds in one directory.
type Collector struct {
	read   ReadIndexedDB
	dir    string
	schema idbmap.Schema
	mapper idbmap.Mapper
}

// NewCollector compiles schema for dir.
//
//	collector, err := idbsource.NewCollector(readIndexedDB, dir, schema)
func NewCollector(read ReadIndexedDB, dir string, schema idbmap.Schema) (*Collector, error) {
	mapper, err := idbmap.NewMapper(schema)
	if err != nil {
		return nil, err
	}
	return &Collector{read: read, dir: dir, schema: schema, mapper: mapper}, nil
}

// CollectEvents reads the directory once, maps it and emits each event.
// Events emitted before a format error stay ingested; the error says the
// application may have changed its format.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	records, err := c.read(c.dir)
	if err != nil {
		return err
	}
	events, tally, err := c.mapper.Apply(records, indexeddb.OriginName(c.dir))
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
	return tally.Check(c.schema)
}
