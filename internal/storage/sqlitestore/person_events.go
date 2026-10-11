package sqlitestore

import (
	"context"
	"fmt"
	"strings"

	"github.com/chipskein/cade/internal/event"
	"github.com/chipskein/cade/internal/listing"
	"github.com/chipskein/cade/internal/storage"
)

var _ storage.PersonIndex = (*Store)(nil)

// authorRoles are the people index roles of who wrote an event.
var authorRoles = literal(string(listing.RoleSender)) + `, ` + literal(string(listing.RoleAuthor))

// PeopleNamed lists the full name keys a typed name matches.
func (s *Store) PeopleNamed(ctx context.Context, name string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT name_key FROM event_people WHERE role IN (`+authorRoles+`)
		AND instr(' ' || name_key || ' ', ?) > 0 ORDER BY name_key`, " "+listing.NameKey(name)+" ")
	if err != nil {
		return nil, fmt.Errorf("find people named %q: %w", name, err)
	}
	return collectStrings(rows)
}

// oneToOneQuery selects the conversations whose every received message
// the person sent; the user's own messages do not count.
var oneToOneQuery = fmt.Sprintf(`SELECT %[1]s FROM events JOIN event_people AS sender
	ON sender.event_id = events.id AND sender.role = %[2]s
	WHERE %[1]s <> '' AND %[3]s <> 'true'
	GROUP BY %[1]s HAVING MIN(sender.name_key = ?) = 1`,
	eventMetadataField(event.ConversationIDKey), literal(string(listing.RoleSender)), eventMetadataField(event.SentByMeKey))

// EventsOfPerson returns the events about the person with the full name
// key key, oldest first.
func (s *Store) EventsOfPerson(ctx context.Context, key string) ([]event.Event, error) {
	first, _, _ := strings.Cut(key, " ")
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events WHERE events.id IN (
		SELECT event_id FROM event_people WHERE (role IN (`+authorRoles+`) AND name_key = ?) OR (role = `+literal(string(listing.RoleMentioned))+` AND name_key = ?))
		OR `+eventMetadataField(event.ConversationIDKey)+` IN (`+oneToOneQuery+`)
		ORDER BY occurred_at, id`, key, first, key)
	if err != nil {
		return nil, fmt.Errorf("read the events of %q: %w", key, err)
	}
	return collectEvents(rows)
}
