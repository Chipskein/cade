package rag

import (
	"context"
	"slices"
	"testing"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/queryplan"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testfakes"
)

func commitOn(uid, author, file string) event.Event {
	return event.Event{UID: uid, Source: event.SourceGit, Timestamp: fixedNow.AddDate(0, 0, -1), Content: "commit " + uid,
		Metadata: event.Commit{Repository: "/src/api", Author: author, Hash: uid, Files: []string{file}}.Metadata()}
}

func imageFile(uid, description string) event.Event {
	metadata := event.File{Path: "/prints/" + uid + ".png"}.Metadata()
	for key, value := range (event.Image{SHA256: uid, Status: event.CaptionDescribed}).Metadata() {
		metadata[key] = value
	}
	return event.Event{UID: uid, Source: event.SourceFile, Timestamp: fixedNow.AddDate(0, 0, -1), Content: "Imagem: " + description, Metadata: metadata}
}

// graphStore holds commits on shared files, images and a message, all
// near the query direction so only the filters decide.
func graphStore() *testfakes.FakeEventStore {
	store := testfakes.NewFakeEventStore()
	store.Events = []event.Event{
		commitOn("rui-retry", "Rui Costa", "erp/retry.go"), commitOn("carla-retry", "Carla Dias", "erp/retry.go"),
		commitOn("carla-other", "Carla Dias", "catalog/cache.go"),
		imageFile("slide", "slide do projeto Orion apresentado por João Mendes"), imageFile("panic", "terminal com panic"),
		teamsMessage("joao-text", "João Mendes", 9),
	}
	store.Embeddings = map[string][]float32{}
	for _, ev := range store.Events {
		store.Embeddings[ev.UID] = []float32{1, 0}
	}
	return store
}

func retrievedUIDs(t *testing.T, query queryplan.Query) []string {
	t.Helper()
	hits, err := restrictedAnswerer(graphStore(), &testfakes.FakeGenerator{}).Retrieve(context.Background(), query, AnswerObserver{})
	if err != nil {
		t.Fatal(err)
	}
	var uids []string
	for _, hit := range hits {
		uids = append(uids, hit.Event.UID)
	}
	slices.Sort(uids)
	return uids
}

func TestRetrieveFindsImagesWhereSomeoneAppears(t *testing.T) {
	query := queryplan.Query{Question: "imagens em que o João aparece", Entities: []string{"João"}, Modality: entity.TypeImage}
	if got := retrievedUIDs(t, query); !slices.Equal(got, []string{"slide"}) {
		t.Fatalf("expected only the slide naming João, not his message or other images, got %v", got)
	}
}

// Regression for the pending "quem mais mexeu no arquivo do retry do ERP
// que o Rui alterou?": the people filter kept only Rui's commits.
func TestRetrieveWidensFromAPersonToWhatTheyChanged(t *testing.T) {
	query := queryplan.Query{Question: "quem mais mexeu no arquivo que o Rui alterou?", Source: event.SourceGit,
		Criteria: listing.Criteria{People: []string{"Rui"}}, Related: true}
	if got := retrievedUIDs(t, query); !slices.Equal(got, []string{"carla-retry", "rui-retry"}) {
		t.Fatalf("expected both commits on erp/retry.go and not Carla's other file, got %v", got)
	}
}

func TestWidenByRelationsKeepsTheFilterWhenNothingRelates(t *testing.T) {
	answerer := restrictedAnswerer(graphStore(), &testfakes.FakeGenerator{})
	filter := storage.EventFilter{To: fixedNow, People: []listing.PersonMatcher{{Name: "joao", AsSender: true}}}
	widened, err := answerer.widenByRelations(context.Background(), filter)
	if err != nil || len(widened.People) != 1 || len(widened.Graph.EntityIDs) != 0 {
		t.Fatalf("expected João's filter kept, he changed nothing, got %+v (err %v)", widened, err)
	}
}

func TestUnknownEntitiesAreReportedAndDropped(t *testing.T) {
	var unknown []string
	observer := AnswerObserver{PeopleResolved: func(_, u []string) { unknown = u }}
	query := queryplan.Query{Question: "imagens da Beatriz", Entities: []string{"Beatriz"}, Modality: entity.TypeImage}
	hits, err := restrictedAnswerer(graphStore(), &testfakes.FakeGenerator{}).Retrieve(context.Background(), query, observer)
	if err != nil || len(hits) != 2 || !slices.Equal(unknown, []string{"Beatriz"}) {
		t.Fatalf("expected both images and Beatriz reported unknown, got %d hits, unknown %v (err %v)", len(hits), unknown, err)
	}
}

// Copies of one picture show the same Image: the best ranked stands for
// them all.
func TestRetrieveFoldsHitsShowingTheSameImage(t *testing.T) {
	store := graphStore()
	copyOfSlide := imageFile("slide-copy", "slide do projeto Orion apresentado por João Mendes")
	copyOfSlide.Metadata[event.ImageSHA256Key] = "slide"
	store.Events = append(store.Events, copyOfSlide)
	store.Embeddings["slide-copy"] = []float32{1, 0}
	query := queryplan.Query{Question: "imagens do João", Entities: []string{"João"}, Modality: entity.TypeImage}
	hits, err := restrictedAnswerer(store, &testfakes.FakeGenerator{}).Retrieve(context.Background(), query, AnswerObserver{})
	if err != nil || len(hits) != 1 || hits[0].Repeats != 1 {
		t.Fatalf("expected one hit standing for both copies, got %+v (err %v)", hits, err)
	}
}
