package idbmap

import (
	"testing"

	"github.com/chipskein/cade/internal/webstore"
)

func TestLocationSelectsItsContainerUnderTheNamespacePrefix(t *testing.T) {
	location := Location{NamespacePrefix: "Teams:profiles:", Container: "profiles"}
	cases := map[webstore.Record]bool{
		{Namespace: "Teams:profiles:u1", Container: "profiles"}:                                       true,
		{Namespace: "Teams:profiles:u1", Container: "people"}:                                         false,
		{Namespace: "Teams:replychain:u1", Container: "profiles"}:                                     false,
		{Namespace: "Teams:profiles:u1", Container: "profiles", DecodeErr: webstore.ErrExternalValue}: false,
	}
	for record, want := range cases {
		if got := location.selects(record); got != want {
			t.Errorf("selects(%+v) = %v, want %v", record, got, want)
		}
	}
}
