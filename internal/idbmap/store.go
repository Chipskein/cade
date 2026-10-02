package idbmap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// schemaFileSuffix ends every saved schema; files with other suffixes in
// the directory (candidates, notes) are not schemas.
const schemaFileSuffix = ".json"

// candidateSuffix marks a regenerated schema awaiting review; it is not
// loaded as a source.
const candidateSuffix = ".candidate" + schemaFileSuffix

// schemaFileMode keeps schemas private like the config: they name the
// user's applications and stores.
const schemaFileMode = 0o600

// SchemaFiles is the file access SchemaDir needs; OSSchemaFiles is the
// real one.
type SchemaFiles interface {
	ListFiles(dir string) ([]string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Exists(path string) bool
}

// OSSchemaFiles reads and writes the real file system.
type OSSchemaFiles struct{}

// ListFiles returns the names of dir's regular files; a missing dir has
// none.
func (OSSchemaFiles) ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list schemas in %q: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func (OSSchemaFiles) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile creates the directory if needed and writes the file.
func (OSSchemaFiles) WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create schema directory for %q: %w", path, err)
	}
	return os.WriteFile(path, data, schemaFileMode)
}

func (OSSchemaFiles) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SchemaDir keeps schemas as <name>.json files, editable by hand.
//
//	dir := idbmap.NewSchemaDir(idbmap.OSSchemaFiles{}, "~/.config/cade/idb-schemas")
//	schemas, err := dir.List()
type SchemaDir struct {
	files SchemaFiles
	path  string
}

func NewSchemaDir(files SchemaFiles, path string) SchemaDir {
	return SchemaDir{files: files, path: path}
}

// ErrSchemaExists is returned by Save when the schema would replace one.
var ErrSchemaExists = errors.New("schema already exists")

// List loads every saved schema, sorted by name. One broken file fails
// the whole list, naming the file, so a hand edit gone wrong is noticed.
func (d SchemaDir) List() ([]Schema, error) {
	names, err := d.Names()
	if err != nil {
		return nil, err
	}
	var schemas []Schema
	for _, name := range names {
		schema, err := d.Load(name)
		if err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

// Names lists the saved schemas without reading them, so a broken file is
// reported when its schema is used, not when any other one is.
func (d SchemaDir) Names() ([]string, error) {
	files, err := d.files.ListFiles(d.path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, file := range files {
		if strings.HasSuffix(file, schemaFileSuffix) && !strings.HasSuffix(file, candidateSuffix) {
			names = append(names, strings.TrimSuffix(file, schemaFileSuffix))
		}
	}
	slices.Sort(names)
	return names, nil
}

// Load reads the schema called name.
func (d SchemaDir) Load(name string) (Schema, error) {
	return d.load(d.FilePath(name))
}

func (d SchemaDir) load(path string) (Schema, error) {
	schema, err := d.readFile(path)
	if err != nil {
		return Schema{}, err
	}
	if want := strings.TrimSuffix(filepath.Base(path), schemaFileSuffix); schema.Name != want {
		return Schema{}, fmt.Errorf("%s: schema named %q, expected %q like its file", path, schema.Name, want)
	}
	return schema, nil
}

// readFile parses the schema at path, whatever the file is called.
func (d SchemaDir) readFile(path string) (Schema, error) {
	raw, err := d.files.ReadFile(path)
	if err != nil {
		return Schema{}, fmt.Errorf("read schema %q: %w", path, err)
	}
	schema, err := Parse(raw)
	if err != nil {
		return Schema{}, fmt.Errorf("%s: %w", path, err)
	}
	return schema, nil
}

// Save writes schema as <name>.json. It refuses to replace an existing
// schema unless overwrite is set, since the user may have edited it.
func (d SchemaDir) Save(schema Schema, overwrite bool) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	path := d.FilePath(schema.Name)
	if !overwrite && d.files.Exists(path) {
		return fmt.Errorf("%w: %q", ErrSchemaExists, path)
	}
	encoded, err := EncodeSchema(schema)
	if err != nil {
		return err
	}
	return d.files.WriteFile(path, encoded)
}

// EncodeSchema writes schema as indented JSON for hand editing, with "<"
// and ">" kept as is: masked path keys like "<id>" must stay readable.
//
//	encoded, err := idbmap.EncodeSchema(schema)
func EncodeSchema(schema Schema) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(schema); err != nil {
		return nil, fmt.Errorf("encode schema %q: %w", schema.Name, err)
	}
	return buffer.Bytes(), nil
}

// FilePath is where the schema called name lives.
func (d SchemaDir) FilePath(name string) string {
	return filepath.Join(d.path, name+schemaFileSuffix)
}
