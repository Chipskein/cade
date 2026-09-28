package leveldbraw

import "testing"

// The Teams cache is a LevelDB written by Chrome in a format nobody
// documents for this use; these fuzzers check that no byte sequence makes
// the reader panic or loop, only fail. Run longer with `go tool mage fuzz`.

func FuzzJournalBatches(f *testing.F) {
	batch := encodeBatch(7, putRecord("k", "v"), deleteRecord("old"))
	f.Add(encodeFragment(fragmentFull, batch))
	f.Add(append(encodeFragment(fragmentFirst, batch[:5]), encodeFragment(fragmentLast, batch[5:])...))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, journal []byte) {
		versions := newVersionSet()
		for _, batch := range journalBatches(journal) {
			_ = decodeBatch(batch, versions)
		}
		versions.liveEntries()
	})
}

func FuzzDecodeBatch(f *testing.F) {
	f.Add(encodeBatch(1, putRecord("key", "value")))
	f.Add(encodeBatch(9, deleteRecord("key"), putRecord("", "")))
	f.Fuzz(func(t *testing.T, batch []byte) {
		_ = decodeBatch(batch, newVersionSet())
	})
}

func FuzzTableEntries(f *testing.F) {
	f.Add(encodeBlock("a", "1", "b", "2"))
	f.Add(make([]byte, 48))
	f.Fuzz(func(t *testing.T, table []byte) {
		_ = tableEntries(table, func(internalKey, value []byte) error {
			_, _, _, err := parseInternalKey(internalKey)
			return err
		})
	})
}

func FuzzBlockEntries(f *testing.F) {
	f.Add(encodeBlock("key", "value", "key2", "value2"))
	f.Fuzz(func(t *testing.T, block []byte) {
		_ = blockEntries(block, func(key, value []byte) error { return nil })
	})
}
