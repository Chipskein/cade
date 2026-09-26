package leveldbraw

import (
	"encoding/binary"
	"fmt"
)

// Block layout: prefix-compressed entries (shared, unshared and value
// lengths as varints), then a restart-point array and its uint32 count.
// Restart points only speed up seeks; a full scan ignores them.
const restartCountLen = 4

func blockEntries(block []byte, visit func(key, value []byte) error) error {
	entries, err := entriesRegion(block)
	if err != nil {
		return err
	}
	var key []byte
	for len(entries) > 0 {
		var value []byte
		key, value, entries, err = nextBlockEntry(entries, key)
		if err != nil {
			return err
		}
		if err := visit(key, value); err != nil {
			return err
		}
	}
	return nil
}

func entriesRegion(block []byte) ([]byte, error) {
	if len(block) < restartCountLen {
		return nil, fmt.Errorf("block of %d bytes lacks its restart count", len(block))
	}
	restarts := int(binary.LittleEndian.Uint32(block[len(block)-restartCountLen:]))
	end := len(block) - restartCountLen - restarts*4
	if restarts < 0 || end < 0 {
		return nil, fmt.Errorf("block of %d bytes claims %d restart points", len(block), restarts)
	}
	return block[:end], nil
}

// nextBlockEntry decodes one entry; the key is rebuilt from the previous
// key's shared prefix, so it is always a fresh slice.
func nextBlockEntry(entries, previousKey []byte) ([]byte, []byte, []byte, error) {
	var lengths [3]uint64
	offset := 0
	for i := range lengths {
		value, width := binary.Uvarint(entries[offset:])
		if width <= 0 {
			return nil, nil, nil, fmt.Errorf("block entry header %x: bad varint", entries[:min(len(entries), 15)])
		}
		lengths[i], offset = value, offset+width
	}
	shared, unshared, valueLen := lengths[0], lengths[1], lengths[2]
	if shared > uint64(len(previousKey)) || uint64(len(entries)-offset) < unshared+valueLen {
		return nil, nil, nil, fmt.Errorf("block entry (shared %d, unshared %d, value %d) overruns its block", shared, unshared, valueLen)
	}
	keyEnd := offset + int(unshared)
	key := append(append([]byte{}, previousKey[:shared]...), entries[offset:keyEnd]...)
	valueEnd := keyEnd + int(valueLen)
	return key, entries[keyEnd:valueEnd], entries[valueEnd:], nil
}
