package sqlitestore

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// graphCorpus mixes what the graph filter must tell apart: images that
// name someone only in their description, a photo and a document attached
// to messages, a message citing a project, and commits on shared files.
func graphCorpus() []event.Event {
	image := func(uid, path, sha, description string) event.Event {
		metadata := event.File{Path: path}.Metadata()
		for key, value := range (event.Image{SHA256: sha, Status: event.CaptionDescribed, Description: description}).Metadata() {
			metadata[key] = value
		}
		return event.Event{UID: uid, Source: event.SourceFile, Timestamp: baseTime, Content: path + "\nImagem: " + description, Metadata: metadata}
	}
	attached := func(uid, attachment string) event.Event {
		message := event.Message{Sender: "Pedro Alves", Conversation: "Pedro Alves", Kind: event.KindOther}
		metadata := message.Metadata()
		metadata[event.AttachmentKey] = attachment
		return event.Event{UID: uid, Source: event.SourceWhatsApp, Timestamp: baseTime.Add(time.Hour), Content: message.Content(), Metadata: metadata}
	}
	commit := func(uid, author, file string) event.Event {
		return event.Event{UID: uid, Source: event.SourceGit, Timestamp: baseTime.Add(2 * time.Hour), Content: "commit " + uid + "\n" + file,
			Metadata: event.Commit{Repository: "/src/api", Author: author, Hash: uid, Files: []string{file}}.Metadata()}
	}
	cited := event.Message{Sender: "Carla Dias", Kind: event.KindChat, Text: "o João mandou o print do Orion"}
	return []event.Event{
		image("img-orion", "/prints/orion.png", "a1", "slide do projeto Orion apresentado por Joao Mendes"),
		image("img-receipt", "/whatsapp/IMG-1.png", "b2", "comprovante de transferência para João Mendes"),
		attached("wa-photo", "IMG-1.JPG"), attached("wa-doc", "contrato.pdf"),
		{UID: "teams-cited", Source: event.SourceTeams, Timestamp: baseTime, Content: cited.Content(), Metadata: cited.Metadata()},
		commit("git-rui", "Rui Costa", "erp/retry.go"), commit("git-carla", "Carla Dias", "erp/retry.go"),
		commit("git-cache", "Rui Costa", "catalog/cache.go"),
	}
}

func storeWithGraphCorpus(t *testing.T) (*Store, []event.Event) {
	t.Helper()
	store, corpus := openTestStore(t), graphCorpus()
	for _, ev := range corpus {
		mustSave(t, store, ev, []float32{1, 0}) // a chunk, so the text is indexed
	}
	return store, corpus
}

// keptInMemory is the reference: GraphFilter.Keeps over the corpus, with
// each event's entity links as the store recorded them.
func keptInMemory(t *testing.T, store *Store, corpus []event.Event, graph storage.GraphFilter) []string {
	t.Helper()
	var kept []string
	for _, ev := range corpus {
		if graph.Keeps(ev, linkedIDs(t, store, ev.UID)) {
			kept = append(kept, ev.UID)
		}
	}
	slices.Sort(kept)
	return kept
}

func linkedIDs(t *testing.T, store *Store, uid string) []int64 {
	t.Helper()
	links, err := store.EntitiesOfEvent(context.Background(), uid)
	testcheck.NoError(t, err)
	var ids []int64
	for _, link := range links {
		ids = append(ids, link.ID)
	}
	return ids
}

func keptInStore(t *testing.T, store *Store, graph storage.GraphFilter) []string {
	t.Helper()
	filter := storage.EventFilter{From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0), Graph: graph}
	events, err := store.EventsMatching(context.Background(), filter)
	testcheck.NoError(t, err)
	kept := uidsOf(events)
	slices.Sort(kept)
	return kept
}

// retryFileID is the entity of erp/retry.go, which two commits changed.
func retryFileID(t *testing.T, store *Store) int64 {
	t.Helper()
	links, err := store.EntitiesOfEvent(context.Background(), "git-rui")
	testcheck.NoError(t, err)
	for _, link := range links {
		if link.Type == entity.TypeFile {
			return link.ID
		}
	}
	t.Fatal("expected git-rui to link its changed file")
	return 0
}

func TestEventsMatchingSelectsWhatTheGraphFilterKeeps(t *testing.T) {
	store, corpus := storeWithGraphCorpus(t)
	filters := map[string]storage.GraphFilter{
		"images":           {Modality: entity.TypeImage},
		"João":             {Appearing: []string{"João"}},
		"projeto do Orion": {Appearing: []string{"projeto do Orion"}},
		"João in images":   {Modality: entity.TypeImage, Appearing: []string{"João"}},
		"João and Pedro":   {Appearing: []string{"João", "Pedro"}},
		"no words":         {Appearing: []string{"Zé"}},
		"retry file":       {EntityIDs: []int64{retryFileID(t, store)}},
	}
	for name, graph := range filters {
		want, got := keptInMemory(t, store, corpus, graph), keptInStore(t, store, graph)
		if !slices.Equal(got, want) {
			t.Errorf("%s: store kept %v, memory %v", name, got, want)
		}
	}
}

// The reference itself, so an agreeing pair cannot both be wrong.
func TestGraphFilterKeepsImagesAndTheNamesInThem(t *testing.T) {
	store, _ := storeWithGraphCorpus(t)
	cases := map[string][]string{
		"images":         keptInStore(t, store, storage.GraphFilter{Modality: entity.TypeImage}),
		"João in images": keptInStore(t, store, storage.GraphFilter{Modality: entity.TypeImage, Appearing: []string{"João"}}),
		"retry file":     keptInStore(t, store, storage.GraphFilter{EntityIDs: []int64{retryFileID(t, store)}}),
	}
	want := map[string][]string{
		"images":         {"img-orion", "img-receipt", "wa-photo"},
		"João in images": {"img-orion", "img-receipt"},
		"retry file":     {"git-carla", "git-rui"},
	}
	for name, got := range cases {
		if !slices.Equal(got, want[name]) {
			t.Errorf("%s: expected %v, got %v", name, want[name], got)
		}
	}
}

func TestResolveAppearingDropsNamesNobodyHas(t *testing.T) {
	store, _ := storeWithGraphCorpus(t)
	scope := storage.EventFilter{From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0), Graph: storage.GraphFilter{Modality: entity.TypeImage}}
	filter, unknown, err := storage.ResolveAppearing(context.Background(), store, scope, []string{"João", "Beatriz", "Zé"})
	testcheck.NoError(t, err)
	if !slices.Equal(filter.Graph.Appearing, []string{"João"}) || !slices.Equal(unknown, []string{"Beatriz", "Zé"}) {
		t.Fatalf("expected João kept and Beatriz and Zé unknown, got %v / %v", filter.Graph.Appearing, unknown)
	}
}
