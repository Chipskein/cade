package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/chipskein/cade/internal/config"
	"github.com/chipskein/cade/internal/idbschema"
	"github.com/chipskein/cade/internal/indexeddb"
	"github.com/chipskein/cade/internal/v8value"
	"github.com/chipskein/cade/internal/webstore"
)

// maxFieldsPerStore keeps the report readable; the most frequent paths are
// the ones an ingestor needs.
const maxFieldsPerStore = 80

// runTeamsSchema prints the masked structure of IndexedDB directories so
// an ingestor can be designed without anyone seeing message content.
func runTeamsSchema(_ context.Context, env commandEnv, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s: cade teams-schema ~/.config/google-chrome/Default/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb",
			env.language.pick("informe o diretório", "name the directory"))
	}
	// IndexedDB readers take no URL scope, so no config is loaded.
	readers, err := env.toolkit.StoreReaders(config.SourcesConfig{})
	if err != nil {
		return err
	}
	for _, dir := range args {
		records, err := readers.ReadKind(webstore.KindIndexedDB, dir)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.stdout, "# %s\n", indexeddb.OriginName(dir))
		for _, summary := range idbschema.Summarize(records) {
			renderStoreSummary(env.stdout, summary, env.language)
		}
	}
	return nil
}

var (
	recordNoun      = nounForms{"registro", "registros", "record", "records"}
	failureNoun     = nounForms{"falha", "falhas", "failed", "failed"}
	blobNoun        = nounForms{"em blob", "em blob", "in a blob", "in blobs"}
	omittedPathNoun = nounForms{"caminho menos frequente omitido", "caminhos menos frequentes omitidos", "less frequent path left out", "less frequent paths left out"}
)

func renderStoreSummary(out io.Writer, summary idbschema.StoreSummary, language Language) {
	fmt.Fprintf(out, language.pick("\nbanco %q · store %q: %s (%s, %s)\n", "\ndatabase %q · store %q: %s (%s, %s)\n"), summary.Database, summary.Store,
		language.count(summary.Records, recordNoun), language.count(summary.Failed, failureNoun), language.count(summary.BlobWrapped, blobNoun))
	fields := mostFrequentFields(summary.Fields, maxFieldsPerStore)
	for _, field := range fields {
		fmt.Fprintf(out, "  %8d  %-22s %s\n", field.Count, describeKinds(field.Kinds), field.Path)
	}
	if omitted := len(summary.Fields) - len(fields); omitted > 0 {
		fmt.Fprintf(out, "  (+%s)\n", language.count(omitted, omittedPathNoun))
	}
}

// mostFrequentFields keeps the top limit fields by count, then restores
// path order so parents stay above children.
func mostFrequentFields(fields []idbschema.FieldStat, limit int) []idbschema.FieldStat {
	if len(fields) <= limit {
		return fields
	}
	ranked := append([]idbschema.FieldStat(nil), fields...)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Count > ranked[j].Count })
	ranked = ranked[:limit]
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Path < ranked[j].Path })
	return ranked
}

func describeKinds(kinds map[v8value.Kind]int) string {
	names := make([]string, 0, len(kinds))
	for kind := range kinds {
		names = append(names, kind.String())
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}
