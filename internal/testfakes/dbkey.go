package testfakes

import (
	"bytes"
	"context"
	"fmt"

	"github.com/chipskein/cade/internal/dbkey"
)

// FakeKeyVault is an in-memory dbkey.KeyVault, as the system's secret store
// would be; Failure, when set, is returned by every call.
type FakeKeyVault struct {
	Key     []byte
	Place   string
	Failure error
}

var _ dbkey.KeyVault = (*FakeKeyVault)(nil)

func (f *FakeKeyVault) Load(context.Context) ([]byte, error) {
	if f.Failure != nil {
		return nil, f.Failure
	}
	if f.Key == nil {
		return nil, fmt.Errorf("nothing in %s: %w", f.Place, dbkey.ErrNoKey)
	}
	return bytes.Clone(f.Key), nil
}

func (f *FakeKeyVault) Location() string {
	return f.Place
}

func (f *FakeKeyVault) Save(_ context.Context, key []byte) error {
	if f.Failure != nil {
		return f.Failure
	}
	f.Key = bytes.Clone(key)
	return nil
}

func (f *FakeKeyVault) Remove(context.Context) error {
	if f.Failure != nil {
		return f.Failure
	}
	f.Key = nil
	return nil
}
