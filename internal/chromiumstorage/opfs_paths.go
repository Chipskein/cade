package chromiumstorage

import (
	"encoding/binary"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/chipskein/cade/internal/leveldbraw"
	"github.com/chipskein/cade/internal/sitestorage"
)

// The Paths database (sandbox_directory_database.cc) keys each entry by
// its decimal id; its value is a base::Pickle of FileInfo: a uint32
// payload size, then parent id (int64), data path and name (each an int32
// length and bytes, padded to 4), modification time. The root is id 0; a
// directory has no data path.
const (
	pickleHeaderLen = 4
	pickleAlignment = 4
	int64Len        = 8
	int32Len        = 4
	rootID          = 0
)

// pathsEntry is one directory or file of the tree.
type pathsEntry struct {
	parent   int64
	dataPath string
	name     string
}

// pathsTree is the entries by id.
type pathsTree map[int64]pathsEntry

func readPathsTree(dir string) (pathsTree, error) {
	entries, err := leveldbraw.ReadLatest(dir)
	if err != nil {
		return nil, fmt.Errorf("read Chromium OPFS tree: %w", err)
	}
	tree := pathsTree{}
	for _, entry := range entries {
		id, err := strconv.ParseInt(string(entry.Key), 10, 64)
		if err != nil {
			continue // CHILD_OF:, LAST_FILE_ID and LAST_INTEGER index the entries
		}
		if tree[id], err = decodeFileInfo(entry.Value); err != nil {
			return nil, fmt.Errorf("entry %d of the Chromium OPFS tree: %w", id, err)
		}
	}
	return tree, nil
}

// files lists the files that have bytes, DiskPath holding the data path
// inside t/; sorted by path, so reads repeat.
func (t pathsTree) files(origin string) []sitestorage.OPFSFile {
	var files []sitestorage.OPFSFile
	for id, entry := range t {
		if id == rootID || entry.dataPath == "" || !filepath.IsLocal(entry.dataPath) {
			continue
		}
		if dir, reachable := t.dirPath(entry.parent); reachable {
			files = append(files, sitestorage.OPFSFile{Origin: origin, Dir: dir, Name: entry.name, DiskPath: entry.dataPath})
		}
	}
	slices.SortFunc(files, func(a, b sitestorage.OPFSFile) int {
		return strings.Compare(path.Join(a.Dir, a.Name), path.Join(b.Dir, b.Name))
	})
	return files
}

// dirPath joins the names from below the root down to id. A parent
// missing or a loop (a damaged database) leaves the file out.
func (t pathsTree) dirPath(id int64) (string, bool) {
	var names []string
	for range len(t) {
		if id == rootID {
			slices.Reverse(names)
			return strings.Join(names, "/"), true
		}
		entry, found := t[id]
		if !found {
			return "", false
		}
		names, id = append(names, entry.name), entry.parent
	}
	return "", false
}

func decodeFileInfo(pickled []byte) (pathsEntry, error) {
	if len(pickled) < pickleHeaderLen || int(binary.LittleEndian.Uint32(pickled)) != len(pickled)-pickleHeaderLen {
		return pathsEntry{}, fmt.Errorf("pickle of %d bytes, expected a uint32 payload size matching the rest", len(pickled))
	}
	payload := pickled[pickleHeaderLen:]
	if len(payload) < int64Len {
		return pathsEntry{}, fmt.Errorf("pickle payload of %d bytes, expected a %d-byte parent id first", len(payload), int64Len)
	}
	entry := pathsEntry{parent: int64(binary.LittleEndian.Uint64(payload))}
	dataPath, rest, err := pickleString(payload[int64Len:])
	if err != nil {
		return pathsEntry{}, fmt.Errorf("data path: %w", err)
	}
	name, _, err := pickleString(rest)
	if err != nil {
		return pathsEntry{}, fmt.Errorf("name: %w", err)
	}
	entry.dataPath, entry.name = dataPath, name
	return entry, nil
}

// pickleString reads an int32 length, the bytes and their padding.
func pickleString(payload []byte) (string, []byte, error) {
	if len(payload) < int32Len {
		return "", nil, fmt.Errorf("%d bytes left, expected a %d-byte length", len(payload), int32Len)
	}
	length := int(int32(binary.LittleEndian.Uint32(payload)))
	padded := (length + pickleAlignment - 1) / pickleAlignment * pickleAlignment
	if length < 0 || len(payload)-int32Len < padded {
		return "", nil, fmt.Errorf("string of %d bytes, expected at most %d left", length, len(payload)-int32Len)
	}
	text := payload[int32Len : int32Len+length]
	return string(text), payload[int32Len+padded:], nil
}
