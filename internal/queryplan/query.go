package queryplan

import (
	"regexp"
	"time"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/textnorm"
	"github.com/chipskein/cade/internal/timeline"
)

// Query is a question resolved into what each part of cade executes: exact
// filters (source, days, criteria, status) that SQLite and the listing
// apply, and SemanticText, the only part matched by meaning.
type Query struct {
	Question   string
	Mode       Mode
	Source     event.Source
	Days       *timeline.DayRange
	Criteria   listing.Criteria
	Topic      string
	TaskStatus TaskStatus
	// SemanticText is what gets embedded; see semanticText.
	SemanticText string
	// OwnCommitsOnly drops commits known to be someone else's: set for
	// first-person questions ("o que eu fiz?") that name no person.
	OwnCommitsOnly bool
	// Folder keeps only files under this absolute path ("" is anywhere).
	Folder string
}

// IsScoped reports whether exact filters narrow the search.
func (q Query) IsScoped() bool {
	return q.Source != "" || q.Days != nil || q.NeedsExactSelection()
}

// NeedsExactSelection reports whether a filter the vector index cannot
// apply (people, direction, folder) narrows the search.
func (q Query) NeedsExactSelection() bool {
	return !q.Criteria.IsEmpty() || q.Folder != ""
}

// Overrides are the user's explicit choices (flags), which win over what
// the model read.
type Overrides struct {
	Source event.Source
	Days   *timeline.DayRange
	// IgnoreQuestion skips reading filters from the question entirely.
	IgnoreQuestion bool
	// DateOrder reads numeric dates in the question (ui.date_order).
	DateOrder timeline.DateOrder
	// Home expands "~" in a folder named by the question.
	Home string
}

// Resolve combines the model's plan with overrides and resolves dates and
// defaults deterministically; the same question and plan always give the
// same Query.
//
//	query := queryplan.Resolve("commits de ontem sobre auth", plan, queryplan.Overrides{}, time.Now())
func Resolve(question string, plan Plan, overrides Overrides, now time.Time) Query {
	if overrides.IgnoreQuestion {
		plan = Plan{}
	}
	query := Query{Question: question, Source: overrides.Source, Days: overrides.Days, Criteria: plan.Criteria,
		Topic: plan.Topic, TaskStatus: plan.TaskStatus}
	if query.Source == "" {
		query.Source = plan.Source
	}
	if query.Days == nil && !overrides.IgnoreQuestion {
		query.Days = ResolvePeriod(question, plan.Period, now, overrides.DateOrder)
	}
	query.Mode = ResolveMode(plan.Mode, query.Days)
	query = withFolder(query, overrides)
	query.SemanticText = semanticText(query)
	query.OwnCommitsOnly = isFirstPerson(question) && len(query.Criteria.People) == 0
	return withTaskDefaults(query, now)
}

// semanticText embeds only the topic when exact filters already cover the
// rest: in "commits de ontem sobre autenticação", "ontem" pulled messages
// saying "ontem" above the auth commits (rank 7 → 2 with the topic alone).
// Unscoped questions keep the whole text: a bare topic sits farther from
// every event and would fall past the max_distance cutoff.
func semanticText(query Query) string {
	if query.Topic != "" && query.IsScoped() {
		return query.Topic
	}
	return query.Question
}

// withFolder restricts the query to the folder the question names; only
// files live in folders, so it implies the file source unless a flag
// chose another.
func withFolder(query Query, overrides Overrides) Query {
	if overrides.IgnoreQuestion {
		return query
	}
	query.Folder = FolderIn(query.Question, overrides.Home)
	if query.Folder != "" && query.Source == "" {
		query.Source = event.SourceFile
	}
	return query
}

// withTaskDefaults gives a period-less task question ("quais tarefas
// finalizei?") today's report, as `cade tasks` does.
func withTaskDefaults(query Query, now time.Time) Query {
	if query.Mode == ModeTasks && query.Days == nil {
		today, _ := timeline.ParseDayRange("hoje", "", now)
		query.Days = &today
	}
	return query
}

// firstPersonCues mark a question about the user's own work. A fixed rule,
// not a planner field: the small model misreads who is asking more often
// than these words do.
var firstPersonCues = regexp.MustCompile(`\b(eu|fiz|fizemos|trabalhei|commitei|comitei|mexi|alterei|corrigi|implementei|meus?|minhas?|i|my|mine)\b`)

func isFirstPerson(question string) bool {
	return firstPersonCues.MatchString(textnorm.Fold(question))
}
