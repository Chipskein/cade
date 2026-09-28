// Package filesource ingests the files of a configured directory (RF1.3).
package filesource

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	pathpkg "path"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest"
)

// Options controls which files are read.
type Options struct {
	// IgnoredDirNames are directory names skipped entirely (e.g. ".git").
	IgnoredDirNames []string
	// MaxFileBytes caps how large a file may be for its text to be read;
	// larger files are still recorded, without content.
	MaxFileBytes     int64
	IgnoredFileGlobs []string
	// Captions holds the images described before this collector runs; an
	// image in it gets its description as text instead of just its name.
	Captions ingest.ImageCaptions
}

// OptionsFor reads the file source settings; captions may be nil.
//
//	opts := filesource.OptionsFor(cfg.Sources, captions)
func OptionsFor(sources config.SourcesConfig, captions ingest.ImageCaptions) Options {
	return Options{IgnoredDirNames: sources.IgnoredDirNames, IgnoredFileGlobs: sources.IgnoredFileGlobs,
		MaxFileBytes: sources.MaxFileBytes, Captions: captions}
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

// SnapshotRoot is the directory this collector reads in full: files stored
// under it and not emitted were removed.
func (c *Collector) SnapshotRoot() string {
	return c.root
}

// CollectEvents emits one event per regular file, keyed by path and
// modification time so that each edit becomes a new timeline entry.
func (c *Collector) CollectEvents(ctx context.Context, emit ingest.EmitFunc) error {
	return Walk(ctx, c.files, c.root, c.opts, func(path string, entry fs.DirEntry) error {
		return c.emitFile(path, entry, emit)
	})
}

// VisitFunc receives a regular file Walk kept, by its path relative to
// the walked root.
type VisitFunc func(path string, entry fs.DirEntry) error

// Walk visits the regular files of files that opts does not ignore. Image
// descriptions walk with it too, so both see the same files (phase 19).
//
//	err := filesource.Walk(ctx, os.DirFS(root), root, opts, visit)
func Walk(ctx context.Context, files fs.FS, root string, opts Options, visit VisitFunc) error {
	return fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %q: %w", filepath.Join(root, path), walkErr)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if entry.IsDir() && path != "." && slices.Contains(opts.IgnoredDirNames, entry.Name()) {
			return fs.SkipDir
		}
		if !entry.Type().IsRegular() || opts.ignoresFile(entry.Name()) {
			return nil
		}
		return visit(path, entry)
	})
}

func (o Options) ignoresFile(name string) bool {
	for _, pattern := range o.IgnoredFileGlobs {
		if matched, _ := pathpkg.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

func (c *Collector) emitFile(path string, entry fs.DirEntry, emit ingest.EmitFunc) error {
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("stat %q: %w", filepath.Join(c.root, path), err)
	}
	absolutePath := filepath.Join(c.root, path)
	if image, described := c.opts.Captions[absolutePath]; described {
		return emit(imageEvent(fileEvent(absolutePath, info, image.SearchableText()), image))
	}
	text, err := c.readText(path, info.Size())
	if err != nil {
		return err
	}
	return emit(fileEvent(absolutePath, info, text))
}

// imageEvent adds the image's metadata to its file event.
func imageEvent(file event.Event, image event.Image) event.Event {
	for key, value := range image.Metadata() {
		file.Metadata[key] = value
	}
	return file
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
		UID:       event.StableID(event.SourceFile, absolutePath),
		Timestamp: modifiedAt,
		Source:    event.SourceFile,
		Content:   content,
		Metadata:  event.File{Path: absolutePath, Size: info.Size(), ModifiedAt: modifiedAt}.Metadata(),
	}
}
