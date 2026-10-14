package testfakes

import (
	"bytes"
	"context"
	"errors"
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

// FakePassphraseTerminal answers each ReadPassphrase with the next of
// Lines and records the prompts; Absent makes it a non-interactive stdin.
type FakePassphraseTerminal struct {
	Absent  bool
	Lines   []string
	Prompts []string
}

var _ dbkey.PassphraseTerminal = (*FakePassphraseTerminal)(nil)

func (f *FakePassphraseTerminal) Interactive() bool {
	return !f.Absent
}

func (f *FakePassphraseTerminal) ReadPassphrase(prompt string) ([]byte, error) {
	f.Prompts = append(f.Prompts, prompt)
	if len(f.Lines) == 0 {
		return nil, errors.New("EOF")
	}
	line := f.Lines[0]
	f.Lines = f.Lines[1:]
	return []byte(line), nil
}
