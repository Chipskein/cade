package firefoxstorage

import (
	"database/sql"
	"encoding/base32"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/sitestorage"
)

// entriesQuery lists every entry with its parent and either a directory
// name or a file name and file id; names are UTF-16 little-endian. The
// root is the entry without a parent.
const entriesQuery = `SELECT e.handle, e.parent, d.name, f.name, m.fileId
FROM Entries e
LEFT JOIN Directories d ON d.handle = e.handle
LEFT JOIN Files f ON f.handle = e.handle
LEFT JOIN MainFiles m ON m.handle = e.handle`

// fileIDEncoding names a file's bytes on disk after its id.
var fileIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// opfsEntry is one directory or file; a file has a file id.
type opfsEntry struct {
	parent string
	name   string
	fileID []byte
}

// opfsTree is the entries by handle.
type opfsTree map[string]opfsEntry

func readTree(db *sql.DB, location string) (opfsTree, error) {
	rows, err := db.Query(entriesQuery)
	if err != nil {
		return nil, fmt.Errorf("list OPFS entries of %q: %w", location, err)
	}
	defer rows.Close()
	tree := opfsTree{}
	for rows.Next() {
		var handle, parent, dirName, fileName, fileID []byte
		if err := rows.Scan(&handle, &parent, &dirName, &fileName, &fileID); err != nil {
			return nil, fmt.Errorf("read OPFS entry of %q: %w", location, err)
		}
		name, err := sitestorage.UTF16LE(append(dirName, fileName...))
		if err != nil {
			return nil, fmt.Errorf("OPFS entry name of %q: %w", location, err)
		}
		tree[string(handle)] = opfsEntry{parent: string(parent), name: name, fileID: fileID}
	}
	return tree, rows.Err()
}

// files lists the files that have bytes, DiskPath holding only the name
// of the file on disk; sorted by path, so reads repeat.
func (t opfsTree) files(origin string) []sitestorage.OPFSFile {
	var files []sitestorage.OPFSFile
	for _, entry := range t {
		if len(entry.fileID) == 0 {
			continue
		}
		dir, reachable := t.dirPath(entry.parent)
		if reachable {
			files = append(files, sitestorage.OPFSFile{Origin: origin, Dir: dir, Name: entry.name, DiskPath: fileIDEncoding.EncodeToString(entry.fileID)})
		}
	}
	slices.SortFunc(files, func(a, b sitestorage.OPFSFile) int {
		return strings.Compare(path.Join(a.Dir, a.Name), path.Join(b.Dir, b.Name))
	})
	return files
}

// dirPath joins the names from below the root down to handle. A parent
// missing or a loop (a damaged database) leaves the file out.
func (t opfsTree) dirPath(handle string) (string, bool) {
	var names []string
	for range len(t) {
		entry, found := t[handle]
		if !found {
			return "", false
		}
		if entry.parent == "" {
			slices.Reverse(names)
			return strings.Join(names, "/"), true
		}
		names, handle = append(names, entry.name), entry.parent
	}
	return "", false
}
