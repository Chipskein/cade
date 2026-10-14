package dbkey

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

// A recovery key is the database key written to be kept outside the
// machine (on paper, in a password manager) and typed back where the
// system's secret store no longer has it: a reinstall, a new TPM. It is
// base32, which survives handwriting (no 0/O or 1/l pairs), with a short
// checksum so a typo is caught before it opens nothing.
const (
	recoveryChecksumSize = 2
	recoveryGroupSize    = 5
	recoveryGroupDivider = "-"
)

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// ErrBadRecoveryKey is wrapped by every error about a recovery key that
// cannot be read back.
var ErrBadRecoveryKey = errors.New("invalid recovery key")

// FormatRecoveryKey writes key as groups of five base32 characters.
//
//	dbkey.FormatRecoveryKey(key) // "CEIRC-EIRCE-..."
func FormatRecoveryKey(key []byte) string {
	encoded := recoveryEncoding.EncodeToString(append(bytes.Clone(key), recoveryChecksum(key)...))
	groups := make([]string, 0, len(encoded)/recoveryGroupSize+1)
	for start := 0; start < len(encoded); start += recoveryGroupSize {
		groups = append(groups, encoded[start:min(start+recoveryGroupSize, len(encoded))])
	}
	return strings.Join(groups, recoveryGroupDivider)
}

// ParseRecoveryKey reads back what FormatRecoveryKey wrote, ignoring case,
// spaces and dividers. The errors never repeat the text: it is a secret.
//
//	key, err := dbkey.ParseRecoveryKey("ceirc-eirce-...")
func ParseRecoveryKey(text string) ([]byte, error) {
	compact := strings.ToUpper(strings.NewReplacer(recoveryGroupDivider, "", " ", "", "\n", "", "\r", "", "\t", "").Replace(text))
	decoded, err := recoveryEncoding.DecodeString(compact)
	if err != nil {
		return nil, fmt.Errorf("%w: %d characters that are not all base32 (A-Z, 2-7); want the key cade encrypt printed", ErrBadRecoveryKey, len(compact))
	}
	if len(decoded) != KeySize+recoveryChecksumSize {
		return nil, fmt.Errorf("%w: %d bytes; want %d, the key cade encrypt printed", ErrBadRecoveryKey, len(decoded), KeySize+recoveryChecksumSize)
	}
	key, checksum := decoded[:KeySize], decoded[KeySize:]
	if !bytes.Equal(checksum, recoveryChecksum(key)) {
		return nil, fmt.Errorf("%w: its checksum does not match; check it for a typo", ErrBadRecoveryKey)
	}
	return key, nil
}

func recoveryChecksum(key []byte) []byte {
	sum := sha256.Sum256(key)
	return sum[:recoveryChecksumSize]
}
