package idbmap

import (
	"fmt"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// UIDChanges pairs, item by item, the UID current gives a message with the
// one next gives it, where they differ: what a store rekey needs so the
// messages already indexed take their new identity instead of coming back
// duplicated. Both schemas must select the same items to be paired.
//
//	changes, err := idbmap.UIDChanges(current, regenerated, records)
func UIDChanges(current, next Schema, records []webstore.Record) (map[string]string, error) {
	if current.Records != next.Records {
		return nil, fmt.Errorf("records %+v and %+v differ, expected the same store and each to pair messages", current.Records, next.Records)
	}
	pair, err := newItemPair(current, next, records)
	if err != nil {
		return nil, err
	}
	changes := map[string]string{}
	for _, record := range records {
		if current.Records.selects(record) {
			pair.collect(record.Value, changes)
		}
	}
	return changes, nil
}

// itemPair maps one item with both schemas.
type itemPair struct {
	current, next               Mapper
	currentLookups, nextLookups map[Field]lookupIndex
}

func newItemPair(current, next Schema, records []webstore.Record) (itemPair, error) {
	var pair itemPair
	var err error
	if pair.current, err = NewMapper(current); err != nil {
		return itemPair{}, err
	}
	if pair.next, err = NewMapper(next); err != nil {
		return itemPair{}, err
	}
	if pair.currentLookups, err = pair.current.buildLookups(records); err != nil {
		return itemPair{}, err
	}
	pair.nextLookups, err = pair.next.buildLookups(records)
	return pair, err
}

func (p itemPair) collect(root *v8value.Value, changes map[string]string) {
	for _, item := range p.current.items(root) {
		before, kept := p.current.keptEvent(item, p.currentLookups)
		after, keptNext := p.next.keptEvent(item, p.nextLookups)
		if kept && keptNext && before.UID != after.UID {
			changes[before.UID] = after.UID
		}
	}
}

// keptEvent maps an item that passes the conditions.
func (m Mapper) keptEvent(item *v8value.Value, lookups map[Field]lookupIndex) (event.Event, bool) {
	if !keepsAll(m.conditions, item) {
		return event.Event{}, false
	}
	return m.mapItem(item, lookups, "")
}
