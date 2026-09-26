// Package filesource ingests the files of a configured directory (RF1.3).
package filesource

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
)

// Options controls which files are read.
type Options struct {
	// IgnoredDirNames are directory names skipped entirely (e.g. ".git").
	IgnoredDirNames []string
	// MaxFileBytes caps how large a file may be for its text to be read;
	// larger files are still recorded, without content.
	MaxFileBytes int64
}

// Collector walks one directory tree.
type Collector struct {
	files fs.FS
	root  string
	opts  Options
}

// NewCollector walks files, labelling paths with root (the absolute
// directory files was opened from).
//
//	collector := filesource.NewCollector(os.DirFS("/home/me/notes"), "/home/me/notes", opts)
func NewCollector(files fs.FS, root string, opts Options) *Collector {
	return &Collector{files: files, root: root, opts: opts}
}

// CollectEvents emits one event per regular file, keyed by path and
// modification time so that each edit becomes a new timeline entry.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	return fs.WalkDir(c.files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		return c.visit(ctx, path, entry, walkErr, emit)
	})
}

func (c *Collector) visit(ctx context.Context, path string, entry fs.DirEntry, walkErr error, emit ingest.EmitFunc) error {
	if walkErr != nil {
		return fmt.Errorf("walk %q: %w", filepath.Join(c.root, path), walkErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if entry.IsDir() && path != "." && slices.Contains(c.opts.IgnoredDirNames, entry.Name()) {
		return fs.SkipDir
	}
	if !entry.Type().IsRegular() {
		return nil
	}
	return c.emitFile(path, entry, emit)
}

func (c *Collector) emitFile(path string, entry fs.DirEntry, emit ingest.EmitFunc) error {
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("stat %q: %w", filepath.Join(c.root, path), err)
	}
	text, err := c.readText(path, info.Size())
	if err != nil {
		return err
	}
	return emit(fileEvent(filepath.Join(c.root, path), info, text))
}

// readText returns "" for files that are too large or not UTF-8 text; they
// are still ingested, only without searchable content.
func (c *Collector) readText(path string, size int64) (string, error) {
	if size == 0 || size > c.opts.MaxFileBytes {
		return "", nil
	}
	raw, err := fs.ReadFile(c.files, path)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", filepath.Join(c.root, path), err)
	}
	if !looksLikeText(raw) {
		return "", nil
	}
	return string(raw), nil
}

func looksLikeText(raw []byte) bool {
	return utf8.Valid(raw) && !bytes.Contains(raw, []byte{0})
}

// fileEvent uses the file name, not the full path, as the first content
// line: long directory prefixes dominated the embedding and made every file
// look alike. The full path stays in metadata.
func fileEvent(absolutePath string, info fs.FileInfo, text string) event.Event {
	modifiedAt := info.ModTime()
	content := filepath.Base(absolutePath)
	if text != "" {
		content += "\n" + text
	}
	return event.Event{
		UID:       event.StableID(event.SourceFile, absolutePath, strconv.FormatInt(modifiedAt.UnixNano(), 10), strconv.FormatInt(info.Size(), 10)),
		Timestamp: modifiedAt,
		Source:    event.SourceFile,
		Content:   content,
		Metadata:  event.File{Path: absolutePath, Size: info.Size()}.Metadata(),
	}
}
