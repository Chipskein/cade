package sitestorage

import (
	"fmt"
	"os"
	"path"

	"github.com/chipskein/cade/internal/jsonvalue"
	"github.com/chipskein/cade/internal/webstore"
)

// MaxFileBytes caps the OPFS files read: a site keeps its JSON state in
// small files, and a larger one is a database or media, not read whole.
const MaxFileBytes = 16 << 20

// OPFSFile is one file of an origin's Origin Private File System, as a
// reader found it: where the site put it and where the browser keeps it.
type OPFSFile struct {
	Origin string
	// Dir is the file's directory inside the origin's file system, its
	// names joined by "/", empty at the root.
	Dir  string
	Name string
	// DiskPath is the file holding its bytes in the profile.
	DiskPath string
}

// Record reads the file into a record: its namespace the directory, its
// container the file name, its value the parsed JSON. A file that is too
// large or not JSON is still a record, with DecodeErr, so checks count it.
//
//	record := sitestorage.OPFSFile{Origin: "https://app.notion.com", Name: "state.json", DiskPath: p}.Record()
func (f OPFSFile) Record() webstore.Record {
	record := webstore.Record{Kind: webstore.KindOPFS, Origin: f.Origin, Namespace: f.Dir, Container: f.Name, Key: path.Join(f.Dir, f.Name)}
	raw, err := readCapped(f.DiskPath)
	if err == nil {
		record.Value, err = jsonvalue.Parse(raw)
	}
	if err != nil {
		record.DecodeErr = fmt.Errorf("OPFS file %q of %s: %w", record.Key, f.Origin, err)
	}
	return record
}

func readCapped(diskPath string) ([]byte, error) {
	info, err := os.Stat(diskPath)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("file of %d bytes, expected at most %d", info.Size(), MaxFileBytes)
	}
	return os.ReadFile(diskPath)
}
