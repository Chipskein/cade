package pdfplan

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/ingest/filesource"
	"github.com/chipskein/cade/internal/pdffile"
	"github.com/chipskein/cade/internal/testfakes"
)

const planRoot = "/docs"

var planTime = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

type planWorld struct {
	store   *testfakes.FakeEventStore
	reader  *testfakes.FakePDFReader
	planner *Planner
}

func newPlanWorld() *planWorld {
	world := &planWorld{store: testfakes.NewFakeEventStore(), reader: &testfakes.FakePDFReader{Documents: map[string]*testfakes.FakePDFDocument{}}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	world.planner = NewPlanner(world.store, world.reader, Settings{MaxBytes: 100, MaxPages: 10}, nil, logger)
	return world
}

func (w *planWorld) plan(t *testing.T, files fstest.MapFS) {
	t.Helper()
	opts := filesource.Options{IgnoredDirNames: []string{"node_modules"}, IgnoredFileGlobs: []string{".env*"}}
	if err := w.planner.PlanDirectory(context.Background(), files, planRoot, opts); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func pdfFile(raw string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(raw), ModTime: planTime}
}

func textPages(texts ...string) *testfakes.FakePDFDocument {
	document := &testfakes.FakePDFDocument{}
	for index, text := range texts {
		document.Pages = append(document.Pages, pdffile.Page{Number: index + 1, Text: text})
	}
	return document
}

func TestPlanReadsPagesWithSeparators(t *testing.T) {
	world := newPlanWorld()
	world.reader.Documents["pdf"] = textPages("um", "", "três")
	world.plan(t, fstest.MapFS{"relatorio.PDF": pdfFile("pdf")})
	reading, found := world.planner.Readings()[planRoot+"/relatorio.PDF"]
	if !found {
		t.Fatalf("readings = %+v", world.planner.Readings())
	}
	if reading.Text != "um\f\ftrês" || reading.PDF != (event.PDF{Status: event.PDFRead, Pages: 3}) {
		t.Errorf("reading = %+v", reading)
	}
	if world.reader.Opened[0].MaxPages != 10 || world.planner.Tally().Read != 1 {
		t.Errorf("opened = %+v, tally = %+v", world.reader.Opened, world.planner.Tally())
	}
}

func TestPlanSeesOnlyWhatTheCollectorSees(t *testing.T) {
	world := newPlanWorld()
	world.plan(t, fstest.MapFS{"notes.md": pdfFile("x"), "node_modules/a.pdf": pdfFile("x"), ".env.pdf": pdfFile("x")})
	if len(world.reader.Opened) != 0 {
		t.Errorf("opened %d files, want none", len(world.reader.Opened))
	}
}

func TestScannedPagesWait(t *testing.T) {
	world := newPlanWorld()
	world.reader.Documents["pdf"] = &testfakes.FakePDFDocument{Pages: []pdffile.Page{{Number: 1, Text: "um"}, {Number: 2, Scanned: true}}}
	world.plan(t, fstest.MapFS{"scan.pdf": pdfFile("pdf")})
	reading := world.planner.Readings()[planRoot+"/scan.pdf"]
	if reading.PDF != (event.PDF{Status: event.PDFPending, Pages: 2, PendingPages: 1}) || reading.Text != "um\f" {
		t.Errorf("reading = %+v", reading)
	}
	if world.planner.Tally().Pending != 1 {
		t.Errorf("tally = %+v", world.planner.Tally())
	}
}

func TestUnreadableFilesNeverStopThePlan(t *testing.T) {
	world := newPlanWorld()
	world.reader.Documents["pdf"] = textPages("ok")
	big := make([]byte, 101)
	world.plan(t, fstest.MapFS{
		"a-big.pdf":     {Data: big, ModTime: planTime},
		"b-corrupt.pdf": pdfFile("corrupt"),
		"c-good.pdf":    pdfFile("pdf"),
	})
	readings := world.planner.Readings()
	for _, name := range []string{"a-big.pdf", "b-corrupt.pdf"} {
		if got := readings[planRoot+"/"+name]; got.PDF.Status != event.PDFUnreadable || got.Text != "" {
			t.Errorf("%s = %+v", name, got)
		}
	}
	if readings[planRoot+"/c-good.pdf"].Text != "ok" || world.planner.Tally().Unreadable != 2 {
		t.Errorf("readings = %+v, tally = %+v", readings, world.planner.Tally())
	}
}

func TestABadPageCostsOnlyThatPage(t *testing.T) {
	world := newPlanWorld()
	document := textPages("um", "dois", "três")
	document.UnreadablePages = map[int]bool{2: true}
	world.reader.Documents["pdf"] = document
	world.plan(t, fstest.MapFS{"a.pdf": pdfFile("pdf")})
	reading := world.planner.Readings()[planRoot+"/a.pdf"]
	if reading.Text != "um\f\ftrês" || reading.PDF.Status != event.PDFRead {
		t.Errorf("reading = %+v", reading)
	}
}

// slowDocument hits the page timeout at once, as a crafted file would.
type slowDocument struct {
	testfakes.FakePDFDocument
}

func (s *slowDocument) Page(context.Context, int) (pdffile.Page, error) {
	s.PageReads++
	return pdffile.Page{}, context.DeadlineExceeded
}

type slowReader struct {
	document *slowDocument
}

func (r slowReader) Open([]byte, pdffile.Limits) (pdffile.Document, error) {
	return r.document, nil
}

func TestAPageThatTimesOutGivesUpOnTheFile(t *testing.T) {
	document := &slowDocument{FakePDFDocument: *textPages("a", "b", "c")}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	planner := NewPlanner(testfakes.NewFakeEventStore(), slowReader{document}, Settings{MaxBytes: 100, MaxPages: 10}, nil, logger)
	err := planner.PlanDirectory(context.Background(), fstest.MapFS{"a.pdf": pdfFile("x")}, planRoot, filesource.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if document.PageReads != 1 || planner.Readings()[planRoot+"/a.pdf"].PDF.Status != event.PDFUnreadable {
		t.Errorf("reads = %d, readings = %+v", document.PageReads, planner.Readings())
	}
}

func TestACancelledRunStopsWithoutMarkingAnything(t *testing.T) {
	world := newPlanWorld()
	world.reader.Documents["pdf"] = textPages("um")
	ctx, cancel := context.WithCancel(context.Background())
	files := fstest.MapFS{"a.pdf": pdfFile("pdf")}
	cancel()
	if err := world.planner.PlanDirectory(ctx, files, planRoot, filesource.Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	document, err := world.reader.Open([]byte("pdf"), pdffile.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := world.planner.readPages(ctx, document, "/docs/a.pdf"); !errors.Is(err, context.Canceled) {
		t.Errorf("readPages: error = %v, want context.Canceled", err)
	}
	if len(world.planner.Readings()) != 0 {
		t.Errorf("readings = %+v", world.planner.Readings())
	}
}

func storedPDF(path string, modified time.Time, status event.PDFStatus) event.Event {
	metadata := event.File{Path: path, ModifiedAt: modified}.Metadata()
	maps.Copy(metadata, event.PDF{Status: status, Pages: 1}.Metadata())
	return event.Event{UID: event.StableID(event.SourceFile, path), Source: event.SourceFile, Metadata: metadata}
}

func TestSettledFilesAreNotReadAgain(t *testing.T) {
	cases := map[string]struct {
		modified time.Time
		status   event.PDFStatus
		reads    bool
	}{
		"read, same version":       {planTime, event.PDFRead, false},
		"unreadable, same version": {planTime, event.PDFUnreadable, false},
		"pending, same version":    {planTime, event.PDFPending, false},
		"stored by name only":      {planTime, event.PDFNone, true},
		"file changed":             {planTime.Add(-time.Hour), event.PDFRead, true},
	}
	for name, test := range cases {
		world := newPlanWorld()
		world.reader.Documents["pdf"] = textPages("um")
		world.store.Events = append(world.store.Events, storedPDF(planRoot+"/a.pdf", test.modified, test.status))
		world.plan(t, fstest.MapFS{"a.pdf": pdfFile("pdf")})
		if got := len(world.reader.Opened) == 1; got != test.reads {
			t.Errorf("%s: read = %v, want %v", name, got, test.reads)
		}
	}
}

func TestAForgottenFileIsNotRead(t *testing.T) {
	world := newPlanWorld()
	world.store.Forgotten = map[string]bool{event.StableID(event.SourceFile, planRoot+"/a.pdf"): true}
	world.plan(t, fstest.MapFS{"a.pdf": pdfFile("pdf")})
	if len(world.reader.Opened) != 0 || len(world.planner.Readings()) != 0 {
		t.Errorf("opened = %+v, readings = %+v", world.reader.Opened, world.planner.Readings())
	}
}

func TestStoreFailureStopsThePlan(t *testing.T) {
	world := newPlanWorld()
	world.store.FailWith = errors.New("disk failure")
	err := world.planner.PlanDirectory(context.Background(), fstest.MapFS{"a.pdf": pdfFile("x")}, planRoot, filesource.Options{})
	if err == nil {
		t.Error("expected the store's failure")
	}
}

// describingWorld has a two-page PDF, the second page a scan, and a describer
// answering with status.
func describingWorld(status event.CaptionStatus, failure error) (*planWorld, *[]string) {
	world := newPlanWorld()
	world.reader.Documents["pdf"] = &testfakes.FakePDFDocument{
		Pages: []pdffile.Page{{Number: 1, Text: "um"}, {Number: 2, Text: "p. 2", Scanned: true}},
		Scans: map[int][]byte{2: []byte("scan")},
	}
	var keys []string
	describe := func(_ context.Context, key string, raw []byte) (event.Image, error) {
		keys = append(keys, key+"="+string(raw))
		return event.Image{SHA256: "h2", Status: status, Description: "um contrato", VisibleText: "CLÁUSULA 1"}, failure
	}
	world.planner.describe = describe
	return world, &keys
}

func TestADescribedPageJoinsTheText(t *testing.T) {
	world, keys := describingWorld(event.CaptionDescribed, nil)
	world.plan(t, fstest.MapFS{"scan.pdf": pdfFile("pdf")})
	reading := world.planner.Readings()[planRoot+"/scan.pdf"]
	want := "um\fp. 2\nImagem: um contrato\nTexto na imagem:\nCLÁUSULA 1"
	if reading.Text != want || reading.PDF != (event.PDF{Status: event.PDFRead, Pages: 2}) {
		t.Errorf("reading = %+v, want text %q", reading, want)
	}
	if len(reading.PageImages) != 1 || reading.PageImages[0].Number != 2 || reading.PageImages[0].Image.SHA256 != "h2" {
		t.Errorf("page images = %+v", reading.PageImages)
	}
	if len(*keys) != 1 || (*keys)[0] != planRoot+"/scan.pdf#p=2=scan" {
		t.Errorf("described = %v", *keys)
	}
}

func TestAPageWaitingForTheLimitKeepsTheFilePending(t *testing.T) {
	world, _ := describingWorld(event.CaptionPending, nil)
	world.plan(t, fstest.MapFS{"scan.pdf": pdfFile("pdf")})
	reading := world.planner.Readings()[planRoot+"/scan.pdf"]
	if reading.PDF != (event.PDF{Status: event.PDFPending, Pages: 2, PendingPages: 1}) || len(reading.PageImages) != 0 {
		t.Errorf("reading = %+v", reading)
	}
}

func TestAnUndecodablePageIsSettledWithoutText(t *testing.T) {
	world, _ := describingWorld(event.CaptionUnreadable, nil)
	world.plan(t, fstest.MapFS{"scan.pdf": pdfFile("pdf")})
	reading := world.planner.Readings()[planRoot+"/scan.pdf"]
	if reading.PDF.Status != event.PDFRead || reading.Text != "um\fp. 2" {
		t.Errorf("reading = %+v", reading)
	}
}

func TestAScanThatCannotBeReadCostsThatPage(t *testing.T) {
	world, keys := describingWorld(event.CaptionDescribed, nil)
	world.reader.Documents["pdf"].Scans = nil
	world.plan(t, fstest.MapFS{"scan.pdf": pdfFile("pdf")})
	if got := world.planner.Readings()[planRoot+"/scan.pdf"]; got.PDF.Status != event.PDFRead || len(*keys) != 0 {
		t.Errorf("reading = %+v, described = %v", got, *keys)
	}
}

func TestAModelFailureStopsTheRun(t *testing.T) {
	failure := errors.New("the model failed")
	world, _ := describingWorld(event.CaptionDescribed, failure)
	err := world.planner.PlanDirectory(context.Background(), fstest.MapFS{"scan.pdf": pdfFile("pdf")}, planRoot, filesource.Options{})
	if !errors.Is(err, failure) {
		t.Errorf("error = %v, want the model's", err)
	}
}

func TestAPendingFileIsReadAgainOnlyWithADescriber(t *testing.T) {
	for name, withDescriber := range map[string]bool{"images off": false, "images on": true} {
		world, _ := describingWorld(event.CaptionDescribed, nil)
		if !withDescriber {
			world.planner.describe = nil
		}
		world.store.Events = append(world.store.Events, storedPDF(planRoot+"/scan.pdf", planTime, event.PDFPending))
		world.plan(t, fstest.MapFS{"scan.pdf": pdfFile("pdf")})
		if got := len(world.reader.Opened) == 1; got != withDescriber {
			t.Errorf("%s: read = %v, want %v", name, got, withDescriber)
		}
	}
}

func TestSettledStatus(t *testing.T) {
	cases := []struct {
		status      event.PDFStatus
		canDescribe bool
		want        bool
	}{
		{event.PDFRead, true, true}, {event.PDFUnreadable, false, true},
		{event.PDFPending, true, false}, {event.PDFPending, false, true}, {event.PDFNone, true, false},
	}
	for _, test := range cases {
		if got := settledStatus(test.status, test.canDescribe); got != test.want {
			t.Errorf("settledStatus(%q, %v) = %v, want %v", test.status, test.canDescribe, got, test.want)
		}
	}
}
