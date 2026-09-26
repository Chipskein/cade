package leveldbraw

import (
	"encoding/binary"
	"fmt"

	"github.com/chipskein/cade/internal/snappyblock"
)

// Table (.ldb/.sst) layout: data blocks, a metaindex and an index block,
// then a 48-byte footer holding the index handle and a magic number. Each
// block is followed by a compression byte and a masked CRC32C.
const (
	tableFooterLen   = 48
	tableMagic       = 0xdb4775248b80fb57
	blockTrailerLen  = 5
	compressionNone  = 0
	compressionSnapp = 1
)

// blockHandle locates a block within the table file.
type blockHandle struct {
	offset uint64
	size   uint64
}

// tableEntries calls visit with every internal key/value of the table.
func tableEntries(table []byte, visit func(internalKey, value []byte) error) error {
	index, err := indexBlock(table)
	if err != nil {
		return err
	}
	return blockEntries(index, func(_, encodedHandle []byte) error {
		handle, _, err := decodeBlockHandle(encodedHandle)
		if err != nil {
			return err
		}
		data, err := readBlock(table, handle)
		if err != nil {
			return err
		}
		return blockEntries(data, visit)
	})
}

func indexBlock(table []byte) ([]byte, error) {
	if len(table) < tableFooterLen {
		return nil, fmt.Errorf("table of %d bytes is smaller than the %d-byte footer", len(table), tableFooterLen)
	}
	footer := table[len(table)-tableFooterLen:]
	if magic := binary.LittleEndian.Uint64(footer[tableFooterLen-8:]); magic != tableMagic {
		return nil, fmt.Errorf("table magic %016x, expected %016x", magic, uint64(tableMagic))
	}
	_, rest, err := decodeBlockHandle(footer)
	if err != nil {
		return nil, fmt.Errorf("metaindex handle: %w", err)
	}
	handle, _, err := decodeBlockHandle(rest)
	if err != nil {
		return nil, fmt.Errorf("index handle: %w", err)
	}
	return readBlock(table, handle)
}

func decodeBlockHandle(encoded []byte) (blockHandle, []byte, error) {
	offset, offsetWidth := binary.Uvarint(encoded)
	if offsetWidth <= 0 {
		return blockHandle{}, nil, fmt.Errorf("block handle %x: bad offset varint", encoded)
	}
	size, sizeWidth := binary.Uvarint(encoded[offsetWidth:])
	if sizeWidth <= 0 {
		return blockHandle{}, nil, fmt.Errorf("block handle %x: bad size varint", encoded)
	}
	return blockHandle{offset: offset, size: size}, encoded[offsetWidth+sizeWidth:], nil
}

func readBlock(table []byte, handle blockHandle) ([]byte, error) {
	end := handle.offset + handle.size + blockTrailerLen
	if end > uint64(len(table)) {
		return nil, fmt.Errorf("block at %d+%d overruns %d-byte table", handle.offset, handle.size, len(table))
	}
	contents := table[handle.offset : handle.offset+handle.size]
	trailer := table[handle.offset+handle.size : end]
	if err := verifyChecksum(trailer[1:], "table block", contents, trailer[:1]); err != nil {
		return nil, fmt.Errorf("block at offset %d: %w", handle.offset, err)
	}
	return decompressBlock(contents, trailer[0])
}

func decompressBlock(contents []byte, compression byte) ([]byte, error) {
	switch compression {
	case compressionNone:
		return contents, nil
	case compressionSnapp:
		return snappyblock.Decode(contents)
	}
	return nil, fmt.Errorf("block compression type %d, expected %d (none) or %d (snappy)", compression, compressionNone, compressionSnapp)
}
