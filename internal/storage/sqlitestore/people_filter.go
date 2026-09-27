package sqlitestore

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

// An EventFilter runs as a condition on the events table, over the people
// index, so filtering reads no content. It must select exactly what
// listing.Select keeps in memory; people_filter_test.go compares the two.

// sqlCondition is a conjunction of SQL fragments and their arguments, in
// placeholder order.
type sqlCondition struct {
	parts []string
	args  []any
}

func (c *sqlCondition) add(part string, args ...any) {
	c.parts, c.args = append(c.parts, part), append(c.args, args...)
}

func (c sqlCondition) String() string {
	if len(c.parts) == 0 {
		return "1"
	}
	return strings.Join(c.parts, " AND ")
}

// filterCondition is filter over the "events" table.
func filterCondition(filter storage.EventFilter) sqlCondition {
	var condition sqlCondition
	addScope(&condition, "events", filter)
	if indexed := filter.Direction.Indexed(); indexed != "" {
		condition.add(`(events.direction IS NULL OR events.direction = ?)`, indexed)
	}
	if filter.Direction == listing.Received {
		addNotAddressedToOthers(&condition, filter)
	}
	if len(filter.People) > 0 {
		addPeople(&condition, filter.People)
	}
	return condition
}

// addScope restricts alias to the filter's period and source.
func addScope(condition *sqlCondition, alias string, filter storage.EventFilter) {
	operator, source := sourceFilter(filter.Source)
	from, to := timeBounds(filter.From, filter.To)
	condition.add(fmt.Sprintf(`%[1]s.occurred_at >= ? AND %[1]s.occurred_at < ? AND %[1]s.source %[2]s ?`, alias, operator),
		from, to, source)
}

// addPeople keeps events where any person appears in one of their roles,
// as a whole-word match of the spelling keys (listing's containsName).
func addPeople(condition *sqlCondition, people []listing.PersonMatcher) {
	var matches []string
	var args []any
	for _, person := range people {
		roles := person.Roles()
		matches = append(matches, `(role IN (`+placeholders(len(roles))+`) AND instr(' ' || name_key || ' ', ?) > 0)`)
		for _, role := range roles {
			args = append(args, string(role))
		}
		args = append(args, " "+person.Key()+" ")
	}
	condition.add(`events.id IN (SELECT event_id FROM event_people WHERE `+strings.Join(matches, " OR ")+`)`, args...)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// addNotAddressedToOthers drops received group messages whose mentions are
// all known people other than the user (listing's addressees).
func addNotAddressedToOthers(condition *sqlCondition, filter storage.EventFilter) {
	known, scope := knownFirstNames(filter)
	self := known + ` AND sender.direction = ` + literal(listing.Sent.Indexed())
	mentioned := literal(string(listing.RoleMentioned))
	condition.add(fmt.Sprintf(`NOT (events.source = %s
		AND EXISTS (SELECT 1 FROM event_people WHERE event_id = events.id AND role = %s)
		AND EXISTS (%s)
		AND NOT EXISTS (SELECT 1 FROM event_people AS mention WHERE mention.event_id = events.id AND mention.role = %s
			AND (mention.name IN (%s) OR mention.name NOT IN (%s))))`,
		literal(string(event.SourceTeams)), mentioned, self, mentioned, self, known),
		slices.Concat(scope.args, scope.args, scope.args)...)
}

// knownFirstNames selects the first names of the Teams senders in the
// filter's scope, as in memory: who is known, and (the messages the user
// sent) which names are the user's. Its arguments are scope's.
func knownFirstNames(filter storage.EventFilter) (string, sqlCondition) {
	var scope sqlCondition
	addScope(&scope, "sender", filter)
	return fmt.Sprintf(`SELECT substr(person.name, 1, instr(person.name || ' ', ' ') - 1)
		FROM event_people AS person JOIN events AS sender ON sender.id = person.event_id
		WHERE person.role = %s AND sender.source = %s AND %s`,
		literal(string(listing.RoleSender)), literal(string(event.SourceTeams)), scope), scope
}

// literal quotes one of this package's constant identifiers; values from
// the user always go through placeholders.
func literal(constant string) string {
	return "'" + constant + "'"
}

// CountMatching counts the events satisfying filter, up to upTo.
func (s *Store) CountMatching(ctx context.Context, filter storage.EventFilter, upTo int) (int, error) {
	condition := filterCondition(filter)
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM events WHERE `+condition.String()+` LIMIT ?)`,
		append(condition.args, upTo)...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events matching %+v: %w", filter, err)
	}
	return count, nil
}

// EventsMatching returns the events satisfying filter, oldest first.
func (s *Store) EventsMatching(ctx context.Context, filter storage.EventFilter) ([]event.Event, error) {
	condition := filterCondition(filter)
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events WHERE `+condition.String()+` ORDER BY occurred_at, id`,
		condition.args...)
	if err != nil {
		return nil, fmt.Errorf("query events matching %+v: %w", filter, err)
	}
	return collectEvents(rows)
}
