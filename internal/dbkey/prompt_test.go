package dbkey_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/dbkey"
	"github.com/chipskein/cade/internal/testfakes"
)

var testPrompts = dbkey.PassphrasePrompts{New: "New passphrase: ", Repeat: "Again: "}

func TestTypedPassphraseIsTheKey(t *testing.T) {
	terminal := &testfakes.FakePassphraseTerminal{Lines: []string{"correct horse battery"}}
	key, err := dbkey.TypedPassphrase{Terminal: terminal, Prompt: "Passphrase: "}.Load(context.Background())
	if err != nil || string(key) != "correct horse battery" || terminal.Prompts[0] != "Passphrase: " {
		t.Errorf("key = %q, %v, prompts %q; want the typed line after the prompt", key, err, terminal.Prompts)
	}
}

func TestTypedPassphraseWithoutATerminalOrALineHoldsNoKey(t *testing.T) {
	for name, terminal := range map[string]*testfakes.FakePassphraseTerminal{"no terminal": {Absent: true}, "empty line": {Lines: []string{""}}} {
		if _, err := (dbkey.TypedPassphrase{Terminal: terminal}).Load(context.Background()); !errors.Is(err, dbkey.ErrNoKey) {
			t.Errorf("%s: err = %v, want dbkey.ErrNoKey", name, err)
		}
	}
}

func TestChoosePassphraseAsksTwice(t *testing.T) {
	terminal := &testfakes.FakePassphraseTerminal{Lines: []string{"correct horse battery", "correct horse battery"}}
	passphrase, err := dbkey.ChoosePassphrase(terminal, testPrompts)
	if err != nil || string(passphrase) != "correct horse battery" || strings.Join(terminal.Prompts, "|") != "New passphrase: |Again: " {
		t.Errorf("passphrase = %q, %v, prompts %q; want it after both prompts", passphrase, err, terminal.Prompts)
	}
}

func TestChoosePassphraseRefuses(t *testing.T) {
	cases := map[string]struct {
		terminal *testfakes.FakePassphraseTerminal
		want     string
	}{
		"no terminal": {&testfakes.FakePassphraseTerminal{Absent: true}, string(dbkey.PassphraseVariable)},
		"too short":   {&testfakes.FakePassphraseTerminal{Lines: []string{"ação curta"}}, "10 characters; want at least " + strconv.Itoa(dbkey.MinPassphraseLength)},
		"mismatch":    {&testfakes.FakePassphraseTerminal{Lines: []string{"correct horse battery", "correct horse batterY"}}, "differ"},
	}
	for name, c := range cases {
		_, err := dbkey.ChoosePassphrase(c.terminal, testPrompts)
		if !errors.Is(err, dbkey.ErrPassphraseRejected) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want dbkey.ErrPassphraseRejected saying %q", name, err, c.want)
		}
	}
}

func TestTerminalPromptOnAFileIsNotInteractive(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	prompt := dbkey.TerminalPrompt{In: file, Out: &strings.Builder{}}
	if prompt.Interactive() {
		t.Error("a regular file is interactive, want false")
	}
	if _, err := prompt.ReadPassphrase("Passphrase: "); err == nil {
		t.Error("reading a passphrase from a file succeeded, want the terminal error")
	}
}
