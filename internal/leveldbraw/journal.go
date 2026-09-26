package leveldbraw

import (
	"encoding/binary"
	"fmt"
)

// Journal (.log) layout: 32 KiB blocks of records, each with a 7-byte header
// (masked CRC32C, uint16 length, type). Large batches span several records.
const (
	journalBlockSize  = 32 * 1024
	journalHeaderLen  = 7
	fragmentFull      = 1
	fragmentFirst     = 2
	fragmentMiddle    = 3
	fragmentLast      = 4
	fragmentZeroBlock = 0
)

// journalFragment is one physical record within a block.
type journalFragment struct {
	kind    byte
	payload []byte
}

// journalBatches reassembles the write batches of a journal. It stops at the
// first torn or corrupt fragment: a browser killed mid-write leaves one at
// the tail, and every batch before it was fully committed.
func journalBatches(journal []byte) [][]byte {
	var batches [][]byte
	var pending []byte
	for _, fragment := range journalFragments(journal) {
		pending = append(pending, fragment.payload...)
		if fragment.kind == fragmentFull || fragment.kind == fragmentLast {
			batches = append(batches, pending)
			pending = nil
		}
	}
	return batches
}

func journalFragments(journal []byte) []journalFragment {
	var fragments []journalFragment
	for start := 0; start < len(journal); start += journalBlockSize {
		block := journal[start:min(start+journalBlockSize, len(journal))]
		blockFragments, intact := blockFragments(block)
		fragments = append(fragments, blockFragments...)
		if !intact {
			return fragments
		}
	}
	return fragments
}

// blockFragments returns the fragments of one block and whether the block
// ended cleanly (false means corruption: stop reading the journal).
func blockFragments(block []byte) ([]journalFragment, bool) {
	var fragments []journalFragment
	for len(block) >= journalHeaderLen {
		fragment, rest, err := readFragment(block)
		if err != nil {
			return fragments, false
		}
		if fragment.kind == fragmentZeroBlock {
			return fragments, true
		}
		fragments, block = append(fragments, fragment), rest
	}
	return fragments, true
}

func readFragment(block []byte) (journalFragment, []byte, error) {
	length := int(binary.LittleEndian.Uint16(block[4:6]))
	kind := block[6]
	if kind == fragmentZeroBlock && length == 0 {
		return journalFragment{kind: kind}, nil, nil
	}
	end := journalHeaderLen + length
	if end > len(block) || kind > fragmentLast {
		return journalFragment{}, nil, fmt.Errorf("fragment type %d of %d bytes overruns %d-byte block", kind, length, len(block))
	}
	payload := block[journalHeaderLen:end]
	if err := verifyChecksum(block[:4], "journal fragment", []byte{kind}, payload); err != nil {
		return journalFragment{}, nil, err
	}
	return journalFragment{kind: kind, payload: payload}, block[end:], nil
}
