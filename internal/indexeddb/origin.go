package indexeddb

import (
	"path/filepath"
	"strings"
)

// Chromium names an origin's IndexedDB directory after the origin; Firefox
// names its parent and calls the directory itself "idb".
const (
	chromiumDirSuffix = ".indexeddb.leveldb"
	firefoxDirName    = "idb"
)

// OriginName names the origin an IndexedDB directory belongs to, without
// the rest of the path (the home directory stays out of events and
// reports).
//
//	indexeddb.OriginName("/p/IndexedDB/https_teams.cloud.microsoft_0.indexeddb.leveldb") == "https_teams.cloud.microsoft_0"
//	indexeddb.OriginName("/p/storage/default/https+++web.whatsapp.com/idb") == "https+++web.whatsapp.com"
func OriginName(dir string) string {
	clean := filepath.Clean(dir)
	if filepath.Base(clean) == firefoxDirName {
		return filepath.Base(filepath.Dir(clean))
	}
	return strings.TrimSuffix(filepath.Base(clean), chromiumDirSuffix)
}
