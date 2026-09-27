package sqlitestore

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
	"github.com/chipskein/cade/internal/testcheck"
)

// filterCorpus mixes what the person filters must tell apart: direct and
// group chats, channel posts, the user's own messages, mentions of others
// and of the user, commits, spelling variants, and other sources.
func filterCorpus() []event.Event {
	chat := func(sender, conversation, kind, text string, sentByMe bool) event.Message {
		return event.Message{Sender: sender, Conversation: conversation, Kind: event.ConversationKind(kind), SentByMe: sentByMe, Text: text}
	}
	messages := []event.Message{
		chat("Ana Prado - Atlas", "Ana Prado - Atlas, Caio Mendes", "chat", "pode revisar o contrato?", false),
		chat("Caio Mendes", "Ana Prado - Atlas, Caio Mendes", "chat", "reviso hoje", true),
		chat("Ana Prado - Atlas", "Atlas › Avisos", "canal", "deploy às 19h", false),
		chat("Ianne Rocha - Atlas", "INTERNO AT - LYRA", "chat", "bom dia", false),
		chat("Mariana Souza", "INTERNO AT - ORION", "chat", "ok", false),
		chat("Marcos Lima - Atlas", "INTERNO", "chat", "ok", false),
		chat("Vitor Alves - Atlas", "INTERNO", "chat", "ok", false),
		chat("Ana Prado - Atlas", "INTERNO", "chat", "@Marcos Lima - Atlas consegue ver?", false),
		chat("Ana Prado - Atlas", "INTERNO", "chat", "@Caio Mendes e @Marcos Lima olhem isso", false),
		chat("Ana Prado - Atlas", "INTERNO", "chat", "@INTERNO AT deploy às 19h", false),
		chat("Willian Costa", "Willian Costa, Caio Mendes", "chat", "segue o link", false),
		chat("Caio Mendes", "Leandro Silva - Atlas, Caio Mendes", "chat", "mandei o relatório", true),
		chat("", "INTERNO", "chat", "mensagem sem remetente", false),
	}
	var events []event.Event
	for i, message := range messages {
		content := message.Sender + ": " + message.Text
		events = append(events, event.Event{UID: fmt.Sprintf("teams-%d", i), Source: event.SourceTeams,
			Timestamp: baseTime.Add(time.Duration(i) * 7 * time.Hour), Content: content, Metadata: message.Metadata()})
	}
	commit := func(uid, author string, offset time.Duration) event.Event {
		return event.Event{UID: uid, Source: event.SourceGit, Timestamp: baseTime.Add(offset), Content: "commit " + uid,
			Metadata: event.Commit{Author: author, Hash: uid}.Metadata()}
	}
	return append(events, commit("git-ana", "Ana Prado", 3*time.Hour), commit("git-leandro", "Leandro Sillva", 30*time.Hour),
		sampleEvent("page", event.SourceBrowser, 5*time.Hour), sampleEvent("note", event.SourceFile, 50*time.Hour))
}

func storeWithCorpus(t *testing.T) (*Store, []event.Event) {
	t.Helper()
	store, corpus := openTestStore(t), filterCorpus()
	for _, ev := range corpus {
		mustSave(t, store, ev, nil)
	}
	return store, corpus
}

// inMemory is the reference: the scope's events, then Criteria.Apply.
func inMemory(corpus []event.Event, scope storage.EventFilter, criteria listing.Criteria) ([]string, []string, []string) {
	var scoped []event.Event
	for _, ev := range corpus {
		inPeriod := !ev.Timestamp.Before(scope.From) && ev.Timestamp.Before(scope.To)
		if inPeriod && (scope.Source == "" || ev.Source == scope.Source) {
			scoped = append(scoped, ev)
		}
	}
	slices.SortStableFunc(scoped, func(a, b event.Event) int { return a.Timestamp.Compare(b.Timestamp) })
	kept, matched, unknown := criteria.Apply(scoped)
	return uidsOf(kept), matched, unknown
}

func inStore(t *testing.T, store *Store, scope storage.EventFilter, criteria listing.Criteria) ([]string, []string, []string) {
	t.Helper()
	filter, resolution, err := storage.ResolveCriteria(context.Background(), store, scope, criteria)
	testcheck.NoError(t, err)
	kept, err := store.EventsMatching(context.Background(), filter)
	testcheck.NoError(t, err)
	return uidsOf(kept), resolution.Matched, resolution.Unknown
}

func uidsOf(events []event.Event) []string {
	var uids []string
	for _, ev := range events {
		uids = append(uids, ev.UID)
	}
	return uids
}

func TestEventsMatchingSelectsWhatApplyKeeps(t *testing.T) {
	store, corpus := storeWithCorpus(t)
	scopes := map[string]storage.EventFilter{
		"all":       {From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0)},
		"first-day": {From: baseTime, To: baseTime.Add(24 * time.Hour)},
		"teams":     {From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0), Source: event.SourceTeams},
		"git":       {From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0), Source: event.SourceGit},
	}
	for name, scope := range scopes {
		t.Run(name, func(t *testing.T) { compareFilters(t, store, corpus, scope) })
	}
}

// compareFilters checks every direction and set of names within scope.
func compareFilters(t *testing.T, store *Store, corpus []event.Event, scope storage.EventFilter) {
	people := [][]string{nil, {"Ana"}, {"Ana Prado"}, {"Ana Souza"}, {"Norteagro"}, {"wilian"}, {"sillva"}, {"Marcos", "Vitor"}, {"Caio"}, {"Leandro Silva"}}
	for _, direction := range []listing.Direction{listing.AnyDirection, listing.Received, listing.Sent} {
		for _, names := range people {
			criteria := listing.Criteria{Direction: direction, People: names}
			wantUIDs, wantMatched, wantUnknown := inMemory(corpus, scope, criteria)
			gotUIDs, gotMatched, gotUnknown := inStore(t, store, scope, criteria)
			if !slices.Equal(gotUIDs, wantUIDs) || !slices.Equal(gotMatched, wantMatched) || !slices.Equal(gotUnknown, wantUnknown) {
				t.Errorf("%+v: store kept %v (matched %v, unknown %v), memory %v (matched %v, unknown %v)",
					criteria, gotUIDs, gotMatched, gotUnknown, wantUIDs, wantMatched, wantUnknown)
			}
		}
	}
}

func TestCountMatchingStopsAtUpTo(t *testing.T) {
	store, _ := storeWithCorpus(t)
	all := storage.EventFilter{From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0)}
	count, err := store.CountMatching(context.Background(), all, 3)
	if err != nil || count != 3 {
		t.Fatalf("expected the count capped at 3, got %d (err %v)", count, err)
	}
}

func TestUpdateEventReindexesPeople(t *testing.T) {
	store, corpus := storeWithCorpus(t)
	edited := corpus[0]
	edited.Metadata = event.Message{Sender: "Bruna Reis", Conversation: "INTERNO", Kind: event.KindChat, Text: "x"}.Metadata()
	testcheck.NoError(t, store.UpdateEvent(context.Background(), edited, nil))
	scope := storage.EventFilter{From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0)}
	for name, want := range map[string]bool{"Bruna": true, "Ana": false} {
		uids, _, _ := inStore(t, store, scope, listing.Criteria{Direction: listing.Received, People: []string{name}})
		if slices.Contains(uids, edited.UID) != want {
			t.Errorf("after the edit, expected %s's received messages to include it: %v, got %v", name, want, uids)
		}
	}
}

func TestSearchSimilarAmongKeepsOnlyMatchingEvents(t *testing.T) {
	store := openTestStore(t)
	ana := event.Event{UID: "ana", Source: event.SourceTeams, Timestamp: baseTime, Content: "Ana: oi",
		Metadata: event.Message{Sender: "Ana Prado", Kind: event.KindChat}.Metadata()}
	rui := event.Event{UID: "rui", Source: event.SourceTeams, Timestamp: baseTime, Content: "Rui: oi",
		Metadata: event.Message{Sender: "Rui Costa", Kind: event.KindChat}.Metadata()}
	mustSave(t, store, rui, []float32{1, 0})
	mustSave(t, store, ana, []float32{0, 1})
	among := storage.EventFilter{People: []listing.PersonMatcher{{Name: "ana", AsSender: true}}}
	hits, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 2, Among: &among})
	if err != nil || len(hits) != 1 || hits[0].Event.UID != "ana" {
		t.Fatalf("expected only Ana's message among the neighbours, got %+v (err %v)", hits, err)
	}
}

// Privacy: `cade forget teams` must leave no Teams name behind in the
// index; the commit authors stay with their commits.
func TestDeleteSourceRemovesPeopleIndex(t *testing.T) {
	store, _ := storeWithCorpus(t)
	_, err := store.DeleteSource(context.Background(), event.SourceTeams)
	testcheck.NoError(t, err)
	rows, err := store.db.Query(`SELECT name FROM event_people ORDER BY name`)
	testcheck.NoError(t, err)
	left, err := collectStrings(rows)
	if err != nil || !slices.Equal(left, []string{"ana prado", "leandro sillva"}) {
		t.Fatalf("expected only the commit authors left in event_people, got %v (err %v)", left, err)
	}
}

// A database from before the index (version 6) gets it built from the
// stored events on open, without re-ingesting.
func TestPeopleIndexMigrationIndexesStoredEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cade.db")
	store, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	for _, ev := range filterCorpus() {
		mustSave(t, store, ev, nil)
	}
	for _, statement := range []string{`DROP TABLE event_people`, `ALTER TABLE events DROP COLUMN direction`, `PRAGMA user_version = 6`} {
		_, err := store.db.Exec(statement)
		testcheck.NoError(t, err)
	}
	store.Close()
	reopened, err := Open(context.Background(), path)
	testcheck.NoError(t, err)
	defer reopened.Close()
	scope := storage.EventFilter{From: time.Unix(0, 0), To: baseTime.AddDate(1, 0, 0)}
	uids, _, _ := inStore(t, reopened, scope, listing.Criteria{Direction: listing.Received, People: []string{"Ana"}})
	if !slices.Equal(uids, []string{"teams-0", "git-ana", "teams-8", "teams-9"}) || versionOf(t, reopened.db) != latestVersion() {
		t.Fatalf("expected Ana's received messages found after the migration, got %v", uids)
	}
}

// rag widens a filtered search up to k = 4096 (maxNeighbours); sqlite-vec
// must accept it.
func TestSearchSimilarAcceptsTheLargestK(t *testing.T) {
	store := openTestStore(t)
	mustSave(t, store, sampleEvent("a", event.SourceGit, 0), []float32{1, 0})
	hits, err := store.SearchSimilar(context.Background(), storage.SimilarityQuery{Embedding: []float32{1, 0}, Limit: 4096})
	if err != nil || len(hits) != 1 {
		t.Fatalf("expected k = 4096 accepted, got %d hits (err %v)", len(hits), err)
	}
}
