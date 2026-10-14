package dbkey

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

var sampleKey = bytes.Repeat([]byte{0x11}, KeySize)

func TestRecoveryKeyRoundTrips(t *testing.T) {
	text := FormatRecoveryKey(sampleKey)
	for _, group := range strings.Split(text, recoveryGroupDivider) {
		if len(group) > recoveryGroupSize {
			t.Fatalf("group %q in %q is longer than %d", group, text, recoveryGroupSize)
		}
	}
	key, err := ParseRecoveryKey(text)
	if err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("parse(%q) = %x, %v; want the key back", text, key, err)
	}
}

func TestParseRecoveryKeyIgnoresCaseSpacesAndDividers(t *testing.T) {
	text := FormatRecoveryKey(sampleKey)
	retyped := " " + strings.ToLower(strings.ReplaceAll(text, recoveryGroupDivider, " ")) + "\n"
	if key, err := ParseRecoveryKey(retyped); err != nil || !bytes.Equal(key, sampleKey) {
		t.Errorf("parse(%q) = %x, %v; want the key back", retyped, key, err)
	}
}

func TestParseRecoveryKeyRejectsWithoutRepeatingTheText(t *testing.T) {
	valid := FormatRecoveryKey(sampleKey)
	typo := "A" + valid[1:]
	if valid[0] == 'A' {
		typo = "B" + valid[1:]
	}
	cases := map[string]struct{ text, want string }{
		"typo":       {typo, "checksum"},
		"short key":  {FormatRecoveryKey(sampleKey[:30]), "32 bytes; want 34"},
		"cut short":  {valid[:20], "10 bytes; want 34"},
		"not base32": {"0189-0189", "not all base32"},
	}
	for name, c := range cases {
		_, err := ParseRecoveryKey(c.text)
		if !errors.Is(err, ErrBadRecoveryKey) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want ErrBadRecoveryKey saying %q", name, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), c.text) {
			t.Errorf("%s: err %q repeats the secret", name, err)
		}
	}
}
