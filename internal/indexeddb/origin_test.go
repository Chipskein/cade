package indexeddb

import "testing"

func TestOriginName(t *testing.T) {
	cases := map[string]string{
		"/x/https_teams.microsoft.com_0.indexeddb.leveldb/":  "https_teams.microsoft.com_0",
		"/p/storage/default/https+++teams.microsoft.com/idb": "https+++teams.microsoft.com",
		"/p/storage/default/https+++web.whatsapp.com/idb/":   "https+++web.whatsapp.com",
		"/home/me/x.leveldb": "x.leveldb",
	}
	for dir, want := range cases {
		if got := OriginName(dir); got != want {
			t.Errorf("OriginName(%q) = %q, want %q", dir, got, want)
		}
	}
}
