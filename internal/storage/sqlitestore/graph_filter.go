package sqlitestore

import (
	"strings"

	"github.com/chipskein/cade/internal/entity"
	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/imagefile"
	"github.com/chipskein/cade/internal/storage"
)

// A GraphFilter runs over the entity links and the text index, so it
// reads no content either. It must select what storage.GraphFilter.Keeps
// keeps in memory; graph_filter_test.go compares the two.

// addGraph narrows condition to graph's modality, names and entities.
func addGraph(condition *sqlCondition, graph storage.GraphFilter) {
	if graph.Modality != "" {
		addModality(condition, graph.Modality)
	}
	for _, name := range graph.Appearing {
		addAppearing(condition, storage.AppearingWords(name))
	}
	if len(graph.EntityIDs) > 0 {
		addLinkedEntities(condition, graph.EntityIDs)
	}
}

// addModality keeps events showing an entity of kind, or, for images,
// messages whose attachment has an image extension.
func addModality(condition *sqlCondition, kind entity.Type) {
	shows := `events.id IN (SELECT link.event_id FROM event_entities AS link JOIN entities ON entities.id = link.entity_id
		WHERE link.role = ` + literal(string(entity.RoleShows)) + ` AND entities.type = ?)`
	if kind != entity.TypeImage {
		condition.add(shows, string(kind))
		return
	}
	attachment := `lower(` + eventMetadataField(event.AttachmentKey) + `)`
	var extensions []string
	args := []any{string(kind)}
	for _, extension := range imagefile.DescribedExtensions {
		extensions = append(extensions, attachment+` LIKE ?`)
		args = append(args, "%"+string(extension))
	}
	condition.add(`(`+shows+` OR `+strings.Join(extensions, " OR ")+`)`, args...)
}

// addAppearing keeps events with a chunk holding every word. The subquery
// does not depend on the outer row, so the text index is searched once.
// A name without words to look for keeps nothing, as in memory.
func addAppearing(condition *sqlCondition, words []string) {
	if len(words) == 0 {
		condition.add(`0`)
		return
	}
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = `"` + word + `"`
	}
	condition.add(`(events.source, events.content_hash) IN (SELECT chunks.source, chunks.content_hash
		FROM chunks_fts JOIN chunks ON chunks.id = chunks_fts.rowid WHERE chunks_fts MATCH ?)`, strings.Join(quoted, " AND "))
}

// addLinkedEntities keeps events linked to any of ids.
func addLinkedEntities(condition *sqlCondition, ids []int64) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	condition.add(`events.id IN (SELECT event_id FROM event_entities WHERE entity_id IN (`+placeholders(len(ids))+`))`, args...)
}
