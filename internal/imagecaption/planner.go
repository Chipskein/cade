package imagecaption

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/ingest"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/llm"
	"github.com/chipskein/cade/internal/storage"
)

// Store is what planning reads from the database.
type Store interface {
	IsForgotten(ctx context.Context, uid string) (bool, error)
	StoredEvent(ctx context.Context, uid string) (event.Event, bool, error)
	storage.ImageCaptionIndex
}

// ClosableDescriber is the vision model; it holds ~2.6 GB until closed.
type ClosableDescriber interface {
	llm.ImageDescriber
	Close() error
}

// DescriberLoader loads the vision model. The planner calls it for the
// first image that needs describing, so a run with nothing new never
// loads it.
type DescriberLoader func() (ClosableDescriber, error)

// Settings are the limits of one run and the model that describes.
type Settings struct {
	MaxImageBytes   int64
	MaxImagesPerRun int
	// Model names the generation model in each description (its file name),
	// so `cade reindex --captions` finds the ones another model wrote.
	Model string
}

// Tally counts what planning did with the images of a run.
type Tally struct {
	Described  int
	Reused     int
	Pending    int
	Unreadable int
}

// outcome is what happened to one image.
type outcome int

const (
	outcomeDescribed outcome = iota
	outcomeReused
	outcomePending
	outcomeUnreadable
)

func (t *Tally) add(result outcome) {
	switch result {
	case outcomeDescribed:
		t.Described++
	case outcomeReused:
		t.Reused++
	case outcomePending:
		t.Pending++
	case outcomeUnreadable:
		t.Unreadable++
	}
}

// Planner decides, image by image, what the file collector will store:
// the first stage of `ingest` with images on (phase 19). It runs before
// the embedder loads, and Close frees the model before it does.
type Planner struct {
	store     Store
	load      DescriberLoader
	settings  Settings
	logger    *slog.Logger
	describer ClosableDescriber
	attempts  int
	captions  ingest.ImageCaptions
	tally     Tally
	progress  func(Tally)
}

// NewPlanner plans against store, loading the model with load when needed.
//
//	planner := imagecaption.NewPlanner(store, loadDescriber, settings, logger)
//	defer planner.Close()
func NewPlanner(store Store, load DescriberLoader, settings Settings, logger *slog.Logger) *Planner {
	return &Planner{store: store, load: load, settings: settings, logger: logger, captions: ingest.ImageCaptions{}, progress: func(Tally) {}}
}

// WithProgress reports the running tally after each image planned.
func (p *Planner) WithProgress(progress func(Tally)) *Planner {
	p.progress = progress
	return p
}

// Captions are the images planned so far, for the file collector.
func (p *Planner) Captions() ingest.ImageCaptions {
	return p.captions
}

// Tally is what the run did so far.
func (p *Planner) Tally() Tally {
	return p.tally
}

// Close frees the model, if it was loaded.
func (p *Planner) Close() error {
	if p.describer == nil {
		return nil
	}
	err := p.describer.Close()
	p.describer = nil
	return err
}

// PlanDirectory plans the images under root (files is root opened), seen
// through the same filters as the file collector.
func (p *Planner) PlanDirectory(ctx context.Context, files fs.FS, root string, opts filesource.Options) error {
	return filesource.Walk(ctx, files, root, opts, func(path string, entry fs.DirEntry) error {
		if !imagefile.IsDescribedImage(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", filepath.Join(root, path), err)
		}
		return p.planImage(ctx, files, filepath.Join(root, path), path, info)
	})
}

func (p *Planner) planImage(ctx context.Context, files fs.FS, absolutePath, path string, info fs.FileInfo) error {
	settled, err := p.alreadySettled(ctx, absolutePath, info)
	if err != nil || settled {
		return err
	}
	image, result, err := p.imageFor(ctx, files, path, info)
	if err != nil {
		return fmt.Errorf("describe image %q: %w", absolutePath, err)
	}
	p.captions[absolutePath] = image
	p.tally.add(result)
	p.progress(p.tally)
	return nil
}

// alreadySettled skips, without reading the file, an image stored at this
// version and already described or found unreadable, and a forgotten one
// (the pipeline would not store it anyway).
func (p *Planner) alreadySettled(ctx context.Context, absolutePath string, info fs.FileInfo) (bool, error) {
	uid := event.StableID(event.SourceFile, absolutePath)
	if forgotten, err := p.store.IsForgotten(ctx, uid); err != nil || forgotten {
		return forgotten, err
	}
	stored, known, err := p.store.StoredEvent(ctx, uid)
	if err != nil || !known {
		return false, err
	}
	revision, _ := stored.Revision()
	status := stored.Image().Status
	return revision == info.ModTime().UnixNano() && (status == event.CaptionDescribed || status == event.CaptionUnreadable), nil
}

func (p *Planner) imageFor(ctx context.Context, files fs.FS, path string, info fs.FileInfo) (event.Image, outcome, error) {
	if info.Size() > p.settings.MaxImageBytes {
		p.logger.Debug("image too large to describe", "path", path, "bytes", info.Size(), "max_bytes", p.settings.MaxImageBytes)
		return event.Image{Status: event.CaptionUnreadable}, outcomeUnreadable, nil
	}
	raw, err := fs.ReadFile(files, path)
	if err != nil {
		return event.Image{}, outcomeUnreadable, err
	}
	hash := sha256Hex(raw)
	reused, found, err := p.store.DescribedImage(ctx, hash)
	if err != nil || found {
		reused.SHA256 = hash
		return reused, outcomeReused, err
	}
	if p.attempts >= p.settings.MaxImagesPerRun {
		return event.Image{SHA256: hash, Status: event.CaptionPending}, outcomePending, nil
	}
	p.attempts++
	return p.describeFile(ctx, path, raw, hash)
}

// describeFile marks an image that does not decode as unreadable, so one
// damaged file never stops the ingestion; a model failure does stop it.
func (p *Planner) describeFile(ctx context.Context, path string, raw []byte, hash string) (event.Image, outcome, error) {
	decoded, err := imagefile.Decode(raw, decodeLimits)
	if err != nil {
		p.logger.Debug("image not decoded", "path", path, "error", err.Error())
		return event.Image{SHA256: hash, Status: event.CaptionUnreadable}, outcomeUnreadable, nil
	}
	caption, err := p.describe(ctx, decoded.Image)
	if err != nil {
		return event.Image{}, outcomeUnreadable, err
	}
	return event.Image{SHA256: hash, Width: decoded.Original.Width, Height: decoded.Original.Height, Status: event.CaptionDescribed,
		Model: p.settings.Model, PromptVersion: PromptVersion, Description: caption.Description, VisibleText: caption.VisibleText}, outcomeDescribed, nil
}

func (p *Planner) describe(ctx context.Context, image llm.RGBImage) (Caption, error) {
	if p.describer == nil {
		describer, err := p.load()
		if err != nil {
			return Caption{}, err
		}
		p.describer = describer
	}
	reply, err := p.describer.DescribeImage(ctx, image, Instructions, MaxTokens)
	if err != nil {
		return Caption{}, err
	}
	return ParseCaption(reply), nil
}

func sha256Hex(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
