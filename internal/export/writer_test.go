package export

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/storage/sqlitestore"
	"github.com/chipskein/cade/internal/testcheck"
	"github.com/chipskein/cade/internal/testfakes"
)

var exportedAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func commitEvent(uid string) event.Event {
	return event.Event{UID: uid, Source: event.SourceGit, Timestamp: exportedAt, Content: "commit " + uid,
		Metadata: event.Commit{Repository: "/src/api", Hash: uid, Author: "Rui Costa", Files: []string{"erp/retry.go"}}.Metadata()}
}

func imageEvent() event.Event {
	metadata := event.File{Path: "/prints/erro.png"}.Metadata()
	for key, value := range (event.Image{SHA256: "h1", Status: event.CaptionDescribed, Model: "m", PromptVersion: 1, Description: "um terminal"}).Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: "img", Source: event.SourceFile, Timestamp: exportedAt.Add(time.Hour), Content: "/prints/erro.png\nImagem: um terminal", Metadata: metadata}
}

func everything() storage.EventFilter {
	return storage.EventFilter{From: time.Unix(0, 0), To: exportedAt.AddDate(1, 0, 0)}
}

// decode reads the export back, one record per line.
func decode(t *testing.T, out *bytes.Buffer) []Record {
	t.Helper()
	var records []Record
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		var record Record
		testcheck.NoError(t, json.Unmarshal(lines.Bytes(), &record))
		records = append(records, record)
	}
	return records
}

func TestWriteExportsEveryStoredEventWithWhatWasDerived(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "cade.db"))
	testcheck.NoError(t, err)
	defer store.Close()
	for _, ev := range []event.Event{commitEvent("c1"), imageEvent()} {
		_, err := store.SaveEvent(context.Background(), ev, nil)
		testcheck.NoError(t, err)
	}
	var out bytes.Buffer
	count, err := Write(context.Background(), store, &out, everything())
	records := decode(t, &out)
	if err != nil || count != 2 || len(records) != 2 {
		t.Fatalf("expected two lines, got %d (%d decoded, err %v)", count, len(records), err)
	}
	commit, image := records[0], records[1]
	if len(commit.People) != 1 || commit.People[0].Name != "rui costa" || len(commit.Relations) != 1 || commit.Relations[0].Relation != string(entity.RelationChanged) {
		t.Fatalf("expected the author and the change in the commit, got %+v", commit)
	}
	if len(image.Representations) != 1 || image.Representations[0].Content != "um terminal" || image.Representations[0].Entity.Key != "h1" {
		t.Fatalf("expected the image's description as a representation, got %+v", image)
	}
}

func TestWritePagesThroughMoreThanAPage(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	for i := range pageSize + 3 {
		ev := commitEvent("c" + time.Duration(i).String())
		ev.Timestamp = exportedAt.Add(time.Duration(i) * time.Minute)
		store.Events = append(store.Events, ev)
	}
	var out bytes.Buffer
	count, err := Write(context.Background(), store, &out, everything())
	if err != nil || count != pageSize+3 || len(decode(t, &out)) != pageSize+3 {
		t.Fatalf("expected every event over two pages, got %d (err %v)", count, err)
	}
}

func TestWriteKeepsOnlyTheFilteredSource(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{commitEvent("c1"), imageEvent()}
	filter := everything()
	filter.Source = event.SourceFile
	var out bytes.Buffer
	if count, err := Write(context.Background(), store, &out, filter); err != nil || count != 1 || decode(t, &out)[0].UID != "img" {
		t.Fatalf("expected only the image, got %d (err %v)", count, err)
	}
}

func TestWriteReportsAStoreFailure(t *testing.T) {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{commitEvent("c1")}
	store.FailWith = errors.New("disk I/O error")
	if _, err := Write(context.Background(), store, &bytes.Buffer{}, everything()); err == nil {
		t.Fatal("expected the store's error")
	}
}
