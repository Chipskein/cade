package chromiumstorage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chipskein/cade/internal/leveldbraw"
	"github.com/chipskein/cade/internal/sitestorage"
	"github.com/chipskein/cade/internal/webstore"
)

// A profile keeps the Origin Private File System of the default bucket in
// "File System" (storage/browser/file_system): the Origins LevelDB maps
// "ORIGIN:" + storage identifier to a numbered directory, whose t/Paths
// LevelDB holds the tree and t/ the files' bytes.
const (
	originsDir       = "Origins"
	sandboxTypeDir   = "t"
	pathsDir         = "Paths"
	originKeyPrefix  = "ORIGIN:"
	identifierSep    = "_"
	defaultPortLabel = "0"
)

// OPFSReader reads the JSON files of one origin's Origin Private File
// System when the origin is in its scope.
type OPFSReader struct {
	scope sitestorage.Scope
}

// NewOPFSReader builds a reader of scope, the configured origins.
//
//	reader := chromiumstorage.NewOPFSReader(scope)
//	records, err := reader.Read("~/.config/google-chrome/Default/File System/007")
func NewOPFSReader(scope sitestorage.Scope) OPFSReader {
	return OPFSReader{scope: scope}
}

func (OPFSReader) Kind() webstore.Kind { return webstore.KindOPFS }

// Recognizes an origin's numbered directory by its tree beside the
// Origins database, which names the origin.
func (OPFSReader) Recognizes(location string) bool {
	paths, err := os.Stat(filepath.Join(location, sandboxTypeDir, pathsDir, leveldbCurrentFile))
	origins, originsErr := os.Stat(filepath.Join(filepath.Dir(filepath.Clean(location)), originsDir))
	return err == nil && paths.Mode().IsRegular() && originsErr == nil && origins.IsDir()
}

// Read returns a record per file, its namespace the file's directory and
// its container the file's name.
func (r OPFSReader) Read(location string) ([]webstore.Record, error) {
	if r.scope.Empty() {
		return nil, sitestorage.ErrEmptyScope
	}
	origin, err := originOfNumberedDir(filepath.Clean(location))
	if err != nil {
		return nil, err
	}
	if !r.scope.Allows(origin) {
		return nil, sitestorage.Refusal(origin, location)
	}
	files, err := readPathsTree(filepath.Join(location, sandboxTypeDir, pathsDir))
	if err != nil {
		return nil, err
	}
	var records []webstore.Record
	for _, file := range files.files(origin) {
		file.DiskPath = filepath.Join(location, sandboxTypeDir, file.DiskPath)
		records = append(records, file.Record())
	}
	return records, nil
}

// originOfNumberedDir finds the origin the Origins database gives the
// numbered directory location.
func originOfNumberedDir(location string) (string, error) {
	entries, err := leveldbraw.ReadLatest(filepath.Join(filepath.Dir(location), originsDir))
	if err != nil {
		return "", fmt.Errorf("read Chromium OPFS origins: %w", err)
	}
	number := filepath.Base(location)
	for _, entry := range entries {
		identifier, isOrigin := strings.CutPrefix(string(entry.Key), originKeyPrefix)
		if isOrigin && string(entry.Value) == number {
			return originOfIdentifier(identifier), nil
		}
	}
	return "", fmt.Errorf("no origin in %s names directory %q, expected an %s<origin> key with that value", originsDir, number, originKeyPrefix)
}

// originOfIdentifier undoes a storage identifier, scheme_host_port with
// port 0 for the scheme's default, e.g. https_teams.microsoft.com_0.
func originOfIdentifier(identifier string) string {
	scheme, rest, found := strings.Cut(identifier, identifierSep)
	cut := strings.LastIndex(rest, identifierSep)
	if !found || cut < 0 {
		return identifier
	}
	host, port := rest[:cut], rest[cut+1:]
	if port == defaultPortLabel {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + port
}
