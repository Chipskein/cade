package leveldbraw

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func encodeFragment(kind byte, payload []byte) []byte {
	header := binary.LittleEndian.AppendUint32(nil, maskedChecksum([]byte{kind}, payload))
	header = binary.LittleEndian.AppendUint16(header, uint16(len(payload)))
	return append(append(header, kind), payload...)
}

func TestJournalBatchesJoinsFragments(t *testing.T) {
	journal := append(encodeFragment(fragmentFirst, []byte("ab")), encodeFragment(fragmentLast, []byte("cd"))...)
	journal = append(journal, encodeFragment(fragmentFull, []byte("ef"))...)
	batches := journalBatches(journal)
	if len(batches) != 2 || string(batches[0]) != "abcd" || string(batches[1]) != "ef" {
		t.Fatalf("expected [abcd ef], got %q", batches)
	}
}

func TestJournalBatchesStopsAtCorruptFragment(t *testing.T) {
	corrupt := encodeFragment(fragmentFull, []byte("bad"))
	corrupt[0] ^= 0xff
	journal := append(encodeFragment(fragmentFull, []byte("ok")), corrupt...)
	if batches := journalBatches(journal); len(batches) != 1 || string(batches[0]) != "ok" {
		t.Fatalf("expected only the intact batch, got %q", batches)
	}
}

func TestJournalBatchesIgnoresTornTail(t *testing.T) {
	journal := append(encodeFragment(fragmentFull, []byte("ok")), encodeFragment(fragmentFull, []byte("torn"))[:9]...)
	if batches := journalBatches(journal); len(batches) != 1 {
		t.Fatalf("expected the torn record to be dropped, got %q", batches)
	}
}

func TestJournalBatchesSkipsZeroPadding(t *testing.T) {
	journal := append(encodeFragment(fragmentFull, []byte("a")), make([]byte, 20)...)
	if batches := journalBatches(journal); len(batches) != 1 {
		t.Fatalf("expected padding to be skipped, got %q", batches)
	}
}

func TestReadFragmentRejectsUnknownKind(t *testing.T) {
	if _, _, err := readFragment(encodeFragment(9, []byte("x"))); err == nil {
		t.Fatal("expected an error for fragment type 9")
	}
}

// encodeBlock builds a block without prefix sharing and one restart point.
func encodeBlock(pairs ...string) []byte {
	var block []byte
	for i := 0; i < len(pairs); i += 2 {
		block = binary.AppendUvarint(block, 0)
		block = binary.AppendUvarint(block, uint64(len(pairs[i])))
		block = binary.AppendUvarint(block, uint64(len(pairs[i+1])))
		block = append(append(block, pairs[i]...), pairs[i+1]...)
	}
	block = binary.LittleEndian.AppendUint32(block, 0)
	return binary.LittleEndian.AppendUint32(block, 1)
}

func TestBlockEntries(t *testing.T) {
	var seen []string
	err := blockEntries(encodeBlock("k1", "v1", "k2", "v2"), func(key, value []byte) error {
		seen = append(seen, string(key)+"="+string(value))
		return nil
	})
	if err != nil || len(seen) != 2 || seen[1] != "k2=v2" {
		t.Fatalf("expected [k1=v1 k2=v2], got %v (err %v)", seen, err)
	}
}

func TestNextBlockEntryRebuildsSharedPrefix(t *testing.T) {
	entry := []byte{3, 1, 1, 'd', 'v'}
	key, value, rest, err := nextBlockEntry(entry, []byte("abcx"))
	if err != nil || string(key) != "abcd" || string(value) != "v" || len(rest) != 0 {
		t.Fatalf("expected abcd=v, got %q=%q (err %v)", key, value, err)
	}
}

func TestNextBlockEntryRejectsOverlongShare(t *testing.T) {
	if _, _, _, err := nextBlockEntry([]byte{9, 0, 0}, []byte("ab")); err == nil {
		t.Fatal("expected an error when sharing more than the previous key")
	}
}

func TestEntriesRegionRejectsTinyBlock(t *testing.T) {
	if _, err := entriesRegion([]byte{1}); err == nil {
		t.Fatal("expected an error for a block without restart count")
	}
}

func TestDecodeBlockHandle(t *testing.T) {
	encoded := binary.AppendUvarint(binary.AppendUvarint(nil, 300), 70)
	handle, rest, err := decodeBlockHandle(append(encoded, 0xAA))
	if err != nil || handle != (blockHandle{offset: 300, size: 70}) || !bytes.Equal(rest, []byte{0xAA}) {
		t.Fatalf("unexpected handle %+v rest %x (err %v)", handle, rest, err)
	}
}

func TestReadBlockVerifiesChecksum(t *testing.T) {
	contents := encodeBlock("k", "v")
	table := append(append([]byte{}, contents...), compressionNone, 0, 0, 0, 0)
	if _, err := readBlock(table, blockHandle{offset: 0, size: uint64(len(contents))}); err == nil {
		t.Fatal("expected a checksum mismatch")
	}
}

func TestReadBlockRejectsOverrun(t *testing.T) {
	if _, err := readBlock(make([]byte, 10), blockHandle{offset: 5, size: 20}); err == nil {
		t.Fatal("expected an overrun error")
	}
}

func TestDecompressBlockRejectsUnknownCompression(t *testing.T) {
	if _, err := decompressBlock([]byte("x"), 7); err == nil {
		t.Fatal("expected an error for compression type 7")
	}
}

func TestVerifyChecksumAcceptsMatching(t *testing.T) {
	stored := binary.LittleEndian.AppendUint32(nil, maskedChecksum([]byte("data")))
	if err := verifyChecksum(stored, "test", []byte("data")); err != nil {
		t.Fatalf("expected matching checksum, got %v", err)
	}
}
