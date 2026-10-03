package testfakes

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
)

// LevelDB journal layout (see leveldbraw): 32 KiB blocks of fragments, each
// a 7-byte header (masked CRC32C, uint16 length, type) and its payload.
const (
	leveldbBlockSize     = 32 * 1024
	leveldbHeaderLen     = 7
	leveldbFragmentFull  = 1
	leveldbFragmentFirst = 2
	leveldbFragmentMid   = 3
	leveldbFragmentLast  = 4
	leveldbValueRecord   = 1
	leveldbCRCMaskDelta  = 0xa282ead8
	leveldbJournalName   = "000003.log"
	leveldbCurrentName   = "CURRENT"
	leveldbManifestName  = "MANIFEST-000001\n"
)

var leveldbCastagnoli = crc32.MakeTable(crc32.Castagnoli)

// LevelDBJournal is a synthetic LevelDB directory holding one write batch
// in its journal, as a browser leaves one between compactions.
type LevelDBJournal struct {
	keys   [][]byte
	values [][]byte
}

// Put adds a key and its value to the batch.
func (j *LevelDBJournal) Put(key, value []byte) {
	j.keys = append(j.keys, key)
	j.values = append(j.values, value)
}

// WriteDir writes the journal and a CURRENT file into dir.
func (j *LevelDBJournal) WriteDir(dir string) error {
	if err := os.WriteFile(filepath.Join(dir, leveldbCurrentName), []byte(leveldbManifestName), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, leveldbJournalName), journalFragments(j.batch()), 0o600)
}

func (j *LevelDBJournal) batch() []byte {
	batch := binary.LittleEndian.AppendUint64(nil, 1)
	batch = binary.LittleEndian.AppendUint32(batch, uint32(len(j.keys)))
	for i, key := range j.keys {
		batch = append(batch, leveldbValueRecord)
		batch = binary.AppendUvarint(batch, uint64(len(key)))
		batch = append(batch, key...)
		batch = binary.AppendUvarint(batch, uint64(len(j.values[i])))
		batch = append(batch, j.values[i]...)
	}
	return batch
}

// journalFragments splits batch across blocks, as LevelDB does for a batch
// larger than what is left of a block.
func journalFragments(batch []byte) []byte {
	var out bytes.Buffer
	first := true
	for first || len(batch) > 0 {
		room := leveldbBlockSize - out.Len()%leveldbBlockSize - leveldbHeaderLen
		if room <= 0 {
			out.Write(make([]byte, room+leveldbHeaderLen))
			continue
		}
		size := min(room, len(batch))
		writeFragment(&out, fragmentType(first, size == len(batch)), batch[:size])
		batch, first = batch[size:], false
	}
	return out.Bytes()
}

func fragmentType(first, last bool) byte {
	switch {
	case first && last:
		return leveldbFragmentFull
	case first:
		return leveldbFragmentFirst
	case last:
		return leveldbFragmentLast
	}
	return leveldbFragmentMid
}

func writeFragment(out *bytes.Buffer, kind byte, payload []byte) {
	crc := crc32.Update(crc32.Update(0, leveldbCastagnoli, []byte{kind}), leveldbCastagnoli, payload)
	_ = binary.Write(out, binary.LittleEndian, ((crc>>15)|(crc<<17))+leveldbCRCMaskDelta)
	_ = binary.Write(out, binary.LittleEndian, uint16(len(payload)))
	out.WriteByte(kind)
	out.Write(payload)
}
