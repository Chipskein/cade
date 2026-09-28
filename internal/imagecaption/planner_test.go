package imagecaption

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/testfakes"
)

const plannedRoot = "/prints"

var planned = time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)

const fakeReply = "Description: um terminal\nVisible text: panic: nil map"

// planWorld is a store, a describer that counts its loads, and a planner
// over them.
type planWorld struct {
	store     *testfakes.FakeEventStore
	describer *testfakes.FakeImageDescriber
	loads     int
	planner   *Planner
}

func newPlanWorld(perRun int) *planWorld {
	world := &planWorld{store: testfakes.NewFakeEventStore(), describer: &testfakes.FakeImageDescriber{Reply: fakeReply}}
	load := func() (ClosableDescriber, error) { world.loads++; return world.describer, nil }
	settings := Settings{MaxImageBytes: 1 << 20, MaxImagesPerRun: perRun, Model: "Qwen3.5-2B-Q4_K_M.gguf"}
	world.planner = NewPlanner(world.store, load, settings, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	return world
}

func (w *planWorld) plan(t *testing.T, files fstest.MapFS) {
	t.Helper()
	opts := filesource.Options{IgnoredDirNames: []string{"node_modules"}, IgnoredFileGlobs: []string{".env*"}}
	if err := w.planner.PlanDirectory(context.Background(), files, plannedRoot, opts); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

// pngFile is a width x height PNG; seed makes files of the same size differ.
func pngFile(t *testing.T, width, height int, seed byte) *fstest.MapFile {
	t.Helper()
	picture := image.NewGray(image.Rect(0, 0, width, height))
	picture.Pix[0] = seed
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: encoded.Bytes(), ModTime: planned}
}

// storedImage is what an earlier run stored for path.
func storedImage(path string, image event.Image) event.Event {
	metadata := event.File{Path: path, ModifiedAt: planned}.Metadata()
	for key, value := range image.Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile, Metadata: metadata}
}

func TestPlanDescribesANewImageWithItsSizeAndModel(t *testing.T) {
	world := newPlanWorld(10)
	world.plan(t, fstest.MapFS{"erro.png": pngFile(t, 40, 20, 1), "notas.md": {Data: []byte("texto")}})
	got := world.planner.Captions()["/prints/erro.png"]
	if got.Status != event.CaptionDescribed || got.Width != 40 || got.Height != 20 || got.Model != "Qwen3.5-2B-Q4_K_M.gguf" ||
		got.PromptVersion != PromptVersion || got.Description != "um terminal" || got.VisibleText != "panic: nil map" || len(got.SHA256) != 64 {
		t.Fatalf("unexpected plan %+v", got)
	}
	if len(world.planner.Captions()) != 1 || world.planner.Tally() != (Tally{Described: 1}) {
		t.Fatalf("expected only the image planned, got %v %+v", world.planner.Captions(), world.planner.Tally())
	}
}

func TestPlanWithoutNewImagesNeverLoadsTheModel(t *testing.T) {
	world := newPlanWorld(10)
	world.plan(t, fstest.MapFS{"notas.md": {Data: []byte("texto")}})
	if world.loads != 0 {
		t.Fatalf("expected no model load, got %d", world.loads)
	}
}

func TestPlanLoadsTheModelOnceForManyImages(t *testing.T) {
	world := newPlanWorld(10)
	world.plan(t, fstest.MapFS{"a.png": pngFile(t, 4, 4, 1), "b.png": pngFile(t, 4, 4, 2), "c.jpg": {Data: []byte("not a jpeg"), ModTime: planned}})
	if world.loads != 1 || len(world.describer.Images) != 2 {
		t.Fatalf("expected one load for two decodable images, got %d loads and %d descriptions", world.loads, len(world.describer.Images))
	}
	if world.planner.Tally() != (Tally{Described: 2, Unreadable: 1}) {
		t.Fatalf("expected the broken jpeg unreadable, got %+v", world.planner.Tally())
	}
}

// A moved or renamed image has the same bytes: its description is reused.
func TestPlanReusesTheDescriptionOfTheSameImage(t *testing.T) {
	world := newPlanWorld(10)
	file := pngFile(t, 8, 8, 7)
	first := newPlanWorld(10)
	first.plan(t, fstest.MapFS{"antigo.png": file})
	world.store.Events = append(world.store.Events, storedImage("/elsewhere/antigo.png", first.planner.Captions()["/prints/antigo.png"]))
	world.plan(t, fstest.MapFS{"renomeado.png": file})
	got := world.planner.Captions()["/prints/renomeado.png"]
	if world.loads != 0 || got.Description != "um terminal" || world.planner.Tally() != (Tally{Reused: 1}) {
		t.Fatalf("expected the stored description reused without the model, got %+v, %d loads", got, world.loads)
	}
}

func TestPlanStopsDescribingAtThePerRunLimit(t *testing.T) {
	world := newPlanWorld(2)
	world.plan(t, fstest.MapFS{"a.png": pngFile(t, 4, 4, 1), "b.png": pngFile(t, 4, 4, 2), "c.png": pngFile(t, 4, 4, 3)})
	if world.planner.Tally() != (Tally{Described: 2, Pending: 1}) || world.planner.Captions()["/prints/c.png"].Status != event.CaptionPending {
		t.Fatalf("expected two described and c pending, got %+v %v", world.planner.Tally(), world.planner.Captions())
	}
}

func TestPlanSkipsSettledAndForgottenImages(t *testing.T) {
	world := newPlanWorld(10)
	world.store.Events = append(world.store.Events, storedImage("/prints/pronto.png", event.Image{Status: event.CaptionDescribed}))
	world.store.Forgotten = map[string]bool{event.StableID(event.SourceFile, "/prints/esquecido.png"): true}
	world.plan(t, fstest.MapFS{"pronto.png": pngFile(t, 4, 4, 1), "esquecido.png": pngFile(t, 4, 4, 2)})
	if len(world.planner.Captions()) != 0 || world.loads != 0 {
		t.Fatalf("expected nothing planned, got %v and %d loads", world.planner.Captions(), world.loads)
	}
}

// An image that waited for the per-run limit is described on a later run,
// though the file did not change.
func TestPlanDescribesAPendingImageOnALaterRun(t *testing.T) {
	world := newPlanWorld(10)
	world.store.Events = append(world.store.Events, storedImage("/prints/depois.png", event.Image{Status: event.CaptionPending}))
	world.plan(t, fstest.MapFS{"depois.png": pngFile(t, 4, 4, 1)})
	if world.planner.Captions()["/prints/depois.png"].Status != event.CaptionDescribed {
		t.Fatalf("expected the pending image described, got %v", world.planner.Captions())
	}
}

func TestPlanRespectsTheFileCollectorFilters(t *testing.T) {
	world := newPlanWorld(10)
	world.plan(t, fstest.MapFS{"node_modules/logo.png": pngFile(t, 4, 4, 1), ".env.png": pngFile(t, 4, 4, 2)})
	if len(world.planner.Captions()) != 0 {
		t.Fatalf("expected ignored folders and globs to apply to images, got %v", world.planner.Captions())
	}
}

func TestPlanMarksATooLargeImageUnreadableWithoutReadingIt(t *testing.T) {
	world := newPlanWorld(10)
	world.planner.settings.MaxImageBytes = 10
	world.plan(t, fstest.MapFS{"foto.jpg": pngFile(t, 40, 40, 1)})
	if got := world.planner.Captions()["/prints/foto.jpg"]; got.Status != event.CaptionUnreadable || got.SHA256 != "" || world.loads != 0 {
		t.Fatalf("expected unreadable without a hash or the model, got %+v", got)
	}
}

func TestPlanStopsWhenTheModelFails(t *testing.T) {
	world := newPlanWorld(10)
	world.describer.FailWith = errors.New("out of memory")
	err := world.planner.PlanDirectory(context.Background(), fstest.MapFS{"a.png": pngFile(t, 4, 4, 1)}, plannedRoot, filesource.Options{})
	if err == nil || !errors.Is(err, world.describer.FailWith) {
		t.Fatalf("expected the model failure to stop planning, got %v", err)
	}
}

func TestCloseFreesALoadedModelOnce(t *testing.T) {
	world := newPlanWorld(10)
	world.plan(t, fstest.MapFS{"a.png": pngFile(t, 4, 4, 1)})
	if err := world.planner.Close(); err != nil || !world.describer.Closed || world.planner.Close() != nil {
		t.Fatalf("expected the model closed, got closed=%v err=%v", world.describer.Closed, err)
	}
}
