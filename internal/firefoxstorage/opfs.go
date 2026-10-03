package firefoxstorage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chipskein/cade/internal/filecopy"
	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/webstore"
)

// An origin's fs directory (dom/fs/parent) holds metadata.sqlite, the tree
// of directories and files, and each file's bytes under its file id in
// base32, in a directory named after the id's first two characters.
const (
	opfsDir         = "fs"
	opfsMetadata    = "metadata.sqlite"
	fileDirNameLen  = 2
	originSeparator = "+++"
	portSeparator   = "+"
)

// OPFSReader reads the JSON files of one origin's Origin Private File
// System when the origin is in its scope.
type OPFSReader struct {
	scope sitestorage.Scope
	open  OpenDatabase
}

// NewOPFSReader builds a reader of scope, the configured origins.
//
//	reader := firefoxstorage.NewOPFSReader(scope, openReadOnly)
//	records, err := reader.Read("~/.floorp/x.default/storage/default/https+++app.notion.com/fs")
func NewOPFSReader(scope sitestorage.Scope, open OpenDatabase) OPFSReader {
	return OPFSReader{scope: scope, open: open}
}

func (OPFSReader) Kind() webstore.Kind { return webstore.KindOPFS }

// Recognizes an origin's fs directory by its name and metadata database.
func (OPFSReader) Recognizes(location string) bool {
	if filepath.Base(filepath.Clean(location)) != opfsDir {
		return false
	}
	info, err := os.Stat(filepath.Join(location, opfsMetadata))
	return err == nil && info.Mode().IsRegular()
}

// Read returns a record per file, its namespace the file's directory and
// its container the file's name.
func (r OPFSReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, sitestorage.ErrEmptyScope
	}
	origin := originOfDir(filepath.Base(filepath.Dir(filepath.Clean(location))))
	if !r.scope.Allows(origin) {
		return nil, sitestorage.Refusal(origin, location)
	}
	tree, err := r.readTree(location)
	if err != nil {
		return nil, err
	}
	var records []webstore.Record
	for _, file := range tree.files(origin) {
		file.DiskPath = filepath.Join(location, file.DiskPath[:fileDirNameLen], file.DiskPath)
		records = append(records, file.Record())
	}
	return records, nil
}

// readTree snapshots the metadata (Firefox keeps it open and writing) and
// reads the tree from the copy; the files themselves are read in place.
func (r OPFSReader) readTree(location string) (opfsTree, error) {
	snapshot, err := os.MkdirTemp("", "cade-firefoxstorage-*")
	if err != nil {
		return nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	defer os.RemoveAll(snapshot)
	if err := filecopy.FlatDirectory(location, snapshot); err != nil {
		return nil, fmt.Errorf("snapshot Firefox OPFS %q: %w", location, err)
	}
	db, err := r.open(filepath.Join(snapshot, opfsMetadata))
	if err != nil {
		return nil, fmt.Errorf("open Firefox OPFS %q: %w", location, err)
	}
	defer db.Close()
	return readTree(db, location)
}

// originOfDir undoes how Firefox names an origin's directory: "://" and
// ":" become "+", e.g. https+++localhost+8080. Attributes after "^" stay,
// so a partitioned or container origin is never in a scope.
func originOfDir(name string) string {
	scheme, host, found := strings.Cut(name, originSeparator)
	if !found {
		return name
	}
	return scheme + "://" + strings.ReplaceAll(host, portSeparator, ":")
}
