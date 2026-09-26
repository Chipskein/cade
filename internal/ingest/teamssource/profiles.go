package teamssource

import (
	"strings"

	"github.com/chipskein/cade/internal/indexeddb"
)

const (
	profileDatabasePrefix = "Teams:profiles:"
	profileStore          = "profiles"
)

// profileNames maps user MRIs ("8:orgid:<guid>") to display names. Some
// cached messages carry only the sender's MRI, not their name.
func profileNames(records []indexeddb.Record) map[string]string {
	names := map[string]string{}
	for _, record := range records {
		if !isStore(record, profileDatabasePrefix, profileStore) {
			continue
		}
		name := strings.TrimSpace(record.Value.Get("displayName").String())
		if mri := record.Value.Get("mri").String(); mri != "" && name != "" {
			names[mri] = name
		}
	}
	return names
}
