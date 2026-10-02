package idbmap

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"testing"
)

// FakeSchemaFiles keeps schema files in memory, keyed by full path.
type FakeSchemaFiles struct {
	Files     map[string][]byte
	FailWrite error
}

func NewFakeSchemaFiles() *FakeSchemaFiles {
	return &FakeSchemaFiles{Files: map[string][]byte{}}
}

func (f *FakeSchemaFiles) ListFiles(dir string) ([]string, error) {
	var names []string
	for file := range f.Files {
		if path.Dir(file) == dir {
			names = append(names, path.Base(file))
		}
	}
	return names, nil
}

func (f *FakeSchemaFiles) ReadFile(file string) ([]byte, error) {
	if raw, ok := f.Files[file]; ok {
		return raw, nil
	}
	return nil, fs.ErrNotExist
}

func (f *FakeSchemaFiles) WriteFile(file string, data []byte) error {
	if f.FailWrite != nil {
		return f.FailWrite
	}
	f.Files[file] = data
	return nil
}

func (f *FakeSchemaFiles) Exists(file string) bool {
	_, ok := f.Files[file]
	return ok
}

const schemaDirPath = "/cfg/idb-schemas"

func savedSchema(t *testing.T) Schema {
	t.Helper()
	schema, err := Parse([]byte(metadataOnlySchema))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	files := NewFakeSchemaFiles()
	dir := NewSchemaDir(files, schemaDirPath)
	if err := dir.Save(savedSchema(t), false); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := dir.Load("whatsapp")
	if err != nil || loaded.Fields[FieldSender].Lookup.Store != "contact" || loaded.Revision != 1 {
		t.Fatalf("expected the saved schema back, got %+v (err %v)", loaded, err)
	}
	if !strings.HasSuffix(string(files.Files[schemaDirPath+"/whatsapp.json"]), "}\n") {
		t.Fatal("expected an indented file ending in a newline, editable by hand")
	}
}

func TestSaveRefusesToReplaceUnlessAsked(t *testing.T) {
	dir := NewSchemaDir(NewFakeSchemaFiles(), schemaDirPath)
	schema := savedSchema(t)
	_ = dir.Save(schema, false)
	if err := dir.Save(schema, false); !errors.Is(err, ErrSchemaExists) {
		t.Fatalf("expected ErrSchemaExists, got %v", err)
	}
	if err := dir.Save(schema, true); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
}

func TestSaveRejectsInvalidSchemasAndWriteErrors(t *testing.T) {
	files := NewFakeSchemaFiles()
	if err := NewSchemaDir(files, schemaDirPath).Save(Schema{Name: "x"}, false); err == nil || len(files.Files) != 0 {
		t.Fatalf("expected an invalid schema to be refused before writing, got %v", err)
	}
	files.FailWrite = errors.New("disk full")
	if err := NewSchemaDir(files, schemaDirPath).Save(savedSchema(t), false); !errors.Is(err, files.FailWrite) {
		t.Fatalf("expected the write error, got %v", err)
	}
}

func TestListSkipsCandidatesAndOtherFiles(t *testing.T) {
	files := NewFakeSchemaFiles()
	dir := NewSchemaDir(files, schemaDirPath)
	_ = dir.Save(savedSchema(t), false)
	files.Files[schemaDirPath+"/whatsapp.candidate.json"] = []byte("{}")
	files.Files[schemaDirPath+"/LEIA-ME.txt"] = []byte("notas")
	files.Files[schemaDirPath+"/history/whatsapp.0.json"] = []byte("{}")
	schemas, err := dir.List()
	if err != nil || len(schemas) != 1 || schemas[0].Name != "whatsapp" {
		t.Fatalf("expected only whatsapp, got %v (err %v)", schemas, err)
	}
}

func TestListNamesTheBrokenFile(t *testing.T) {
	files := NewFakeSchemaFiles()
	files.Files[schemaDirPath+"/quebrado.json"] = []byte(`{"version": 1`)
	_, err := NewSchemaDir(files, schemaDirPath).List()
	if err == nil || !strings.Contains(err.Error(), "quebrado.json") {
		t.Fatalf("expected the error to name the file, got %v", err)
	}
}

func TestLoadRequiresTheNameToMatchTheFile(t *testing.T) {
	files := NewFakeSchemaFiles()
	files.Files[schemaDirPath+"/outro.json"] = []byte(metadataOnlySchema)
	if _, err := NewSchemaDir(files, schemaDirPath).Load("outro"); err == nil || !strings.Contains(err.Error(), `expected "outro"`) {
		t.Fatalf("expected a name mismatch error, got %v", err)
	}
}

func TestLoadMissingSchema(t *testing.T) {
	if _, err := NewSchemaDir(NewFakeSchemaFiles(), schemaDirPath).Load("nada"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not exist, got %v", err)
	}
}

func TestOSSchemaFilesRoundTrip(t *testing.T) {
	dir := NewSchemaDir(OSSchemaFiles{}, t.TempDir()+"/nested")
	if schemas, err := dir.List(); err != nil || len(schemas) != 0 {
		t.Fatalf("missing directory: expected no schemas, got %v (err %v)", schemas, err)
	}
	if err := dir.Save(savedSchema(t), false); err != nil {
		t.Fatalf("save: %v", err)
	}
	if schemas, err := dir.List(); err != nil || len(schemas) != 1 {
		t.Fatalf("expected the saved schema, got %v (err %v)", schemas, err)
	}
}
