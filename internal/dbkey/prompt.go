package dbkey

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"golang.org/x/term"
)

// MinPassphraseLength is the shortest passphrase cade encrypt accepts, in
// characters. sqlite3mc stretches it with PBKDF2, but a short one is still
// guessed offline from a copy of the file.
const MinPassphraseLength = 12

// PassphraseTerminal reads a line without echoing it; TerminalPrompt is
// the real one.
type PassphraseTerminal interface {
	// Interactive reports whether there is a terminal to ask on: not
	// under the ingest timer, nor with stdin redirected.
	Interactive() bool
	// ReadPassphrase writes prompt and reads one line, not echoed.
	ReadPassphrase(prompt string) ([]byte, error)
}

// TerminalPrompt asks on In, the terminal, writing the prompt to Out
// (stderr, so it stays out of piped output).
type TerminalPrompt struct {
	In  *os.File
	Out io.Writer
}

func (p TerminalPrompt) Interactive() bool {
	return term.IsTerminal(int(p.In.Fd()))
}

func (p TerminalPrompt) ReadPassphrase(prompt string) ([]byte, error) {
	fmt.Fprint(p.Out, prompt)
	passphrase, err := term.ReadPassword(int(p.In.Fd()))
	// The Enter that ended the line was not echoed either.
	fmt.Fprintln(p.Out)
	if err != nil {
		return nil, fmt.Errorf("read the passphrase from the terminal: %w", err)
	}
	return passphrase, nil
}

// TypedPassphrase is the KeySource of a passphrase typed when cade opens
// the database, where there is no secret store and no variable.
type TypedPassphrase struct {
	Terminal PassphraseTerminal
	Prompt   string
}

// Load asks once; no terminal or an empty line is ErrNoKey.
func (t TypedPassphrase) Load(context.Context) ([]byte, error) {
	if !t.Terminal.Interactive() {
		return nil, fmt.Errorf("no terminal to type the passphrase on: %w", ErrNoKey)
	}
	passphrase, err := t.Terminal.ReadPassphrase(t.Prompt)
	if err != nil {
		return nil, err
	}
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("an empty passphrase was typed: %w", ErrNoKey)
	}
	return passphrase, nil
}

func (t TypedPassphrase) Location() string {
	return "a passphrase typed at the terminal"
}

// PassphrasePrompts are the two questions ChoosePassphrase asks, in the
// CLI's language.
type PassphrasePrompts struct {
	New, Repeat string
}

// ErrPassphraseRejected is wrapped by every reason ChoosePassphrase refuses.
var ErrPassphraseRejected = errors.New("passphrase rejected")

// ChoosePassphrase asks for a new passphrase twice, for cade encrypt where
// there is no secret store.
//
//	passphrase, err := dbkey.ChoosePassphrase(terminal, prompts)
func ChoosePassphrase(terminal PassphraseTerminal, prompts PassphrasePrompts) ([]byte, error) {
	if !terminal.Interactive() {
		return nil, fmt.Errorf("%w: no terminal to type it on; set %s instead", ErrPassphraseRejected, PassphraseVariable)
	}
	passphrase, err := terminal.ReadPassphrase(prompts.New)
	if err != nil {
		return nil, err
	}
	if length := utf8.RuneCount(passphrase); length < MinPassphraseLength {
		return nil, fmt.Errorf("%w: %d characters; want at least %d", ErrPassphraseRejected, length, MinPassphraseLength)
	}
	repeated, err := terminal.ReadPassphrase(prompts.Repeat)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(passphrase, repeated) {
		return nil, fmt.Errorf("%w: the two passphrases differ", ErrPassphraseRejected)
	}
	return passphrase, nil
}
