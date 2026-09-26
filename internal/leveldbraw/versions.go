// Package leveldbraw reads every table and journal of a LevelDB directory
// and keeps, per key, the version with the highest sequence number.
//
// It deliberately avoids LevelDB's merging iterator: Chromium's IndexedDB
// databases are ordered by a custom comparator ("idb_cmp1") that goleveldb
// does not implement, and merging with the wrong order can resurrect stale
// or deleted values. "Highest sequence wins" is correct under any ordering.
package leveldbraw

import (
	"bytes"
	"sort"
)

// Entry is the newest live value of one key.
type Entry struct {
	Key   []byte
	Value []byte
}

type keyVersion struct {
	sequence uint64
	deleted  bool
	value    []byte
}

// versionSet accumulates versions of each key, keeping the newest.
type versionSet struct {
	latest map[string]keyVersion
}

func newVersionSet() *versionSet {
	return &versionSet{latest: map[string]keyVersion{}}
}

func (v *versionSet) record(key []byte, sequence uint64, deleted bool, value []byte) {
	current, seen := v.latest[string(key)]
	if seen && current.sequence >= sequence {
		return
	}
	v.latest[string(key)] = keyVersion{sequence: sequence, deleted: deleted, value: bytes.Clone(value)}
}

// liveEntries returns non-deleted keys, sorted bytewise for deterministic
// output.
func (v *versionSet) liveEntries() []Entry {
	entries := make([]Entry, 0, len(v.latest))
	for key, version := range v.latest {
		if !version.deleted {
			entries = append(entries, Entry{Key: []byte(key), Value: version.value})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].Key, entries[j].Key) < 0 })
	return entries
}
