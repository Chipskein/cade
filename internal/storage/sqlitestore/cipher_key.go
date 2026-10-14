package sqlitestore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/mattn/go-sqlite3"
)

// CipherKey encrypts the database with SQLite3 Multiple Ciphers (#111). An
// empty key opens it in plain text, as before #111.
type CipherKey []byte

// ErrCipherKeyRejected is wrapped by every error about a key that does not
// open the file; the message says which case it is.
var ErrCipherKeyRejected = errors.New("database key rejected")

const (
	// The key goes in the URI, so sqlite3mc applies it while opening: the
	// driver's PRAGMAs (journal_mode) run before a ConnectHook could, and
	// would fail on the still encrypted file (#57). Hex keeps any byte
	// out of the URI's syntax.
	hexKeyParam = "&hexkey="
	// cipherParam pins the cipher: the file does not record it, so a new
	// default in a later sqlite3mc would not open older databases.
	cipherParam = "&cipher=chacha20"
	// encryptedCacheKiB is the page cache when encrypted: each page read
	// from disk is decrypted, and with SQLite's 2 MB a search rereads them
	// (2 to 10 times slower in #57); 256 MB made the warm cost the plain one.
	encryptedCacheKiB = 262144
	// cacheSizeParam is negative, in KiB, as PRAGMA cache_size takes it.
	cacheSizeParam = "&_cache_size=-"
	// plainHeader starts every unencrypted SQLite file.
	plainHeader = "SQLite format 3\x00"
)

// databaseDSN is the URI that opens path with the driver params and key.
//
//	databaseDSN("/x/cade.db", "mode=ro", nil) // "file:/x/cade.db?mode=ro"
func databaseDSN(path, params string, key CipherKey) string {
	dsn := "file:" + path + "?" + params
	if len(key) == 0 {
		return dsn
	}
	return dsn + cipherParam + hexKeyParam + hex.EncodeToString(key) + cacheSizeParam + strconv.Itoa(encryptedCacheKiB)
}

// openKeyed opens path and connects once, so a key that does not match the
// file fails here, before any statement, and without writing to it.
func openKeyed(ctx context.Context, path, params string, key CipherKey) (*sql.DB, error) {
	db, err := sql.Open(DriverName, databaseDSN(path, params, key))
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, explainOpenFailure(path, key, err)
	}
	return db, nil
}

// explainOpenFailure turns SQLite's "file is not a database", which is
// what a missing, wrong or unneeded key gives, into which of them it is.
func explainOpenFailure(path string, key CipherKey, err error) error {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code != sqlite3.ErrNotADB {
		return fmt.Errorf("open database %q: %w", path, err)
	}
	plain, headerErr := hasPlainHeader(path)
	switch {
	case headerErr != nil:
		return fmt.Errorf("open database %q: %w (reading its header: %v)", path, err, headerErr)
	case plain:
		return fmt.Errorf("database %q is in plain text, but a key was given; want no key: %w", path, ErrCipherKeyRejected)
	case len(key) == 0:
		return fmt.Errorf("database %q is encrypted (or not a SQLite file) and no key was given; want the key it was encrypted with: %w", path, ErrCipherKeyRejected)
	}
	return fmt.Errorf("database %q is encrypted (or not a SQLite file) and the key given does not decrypt it; want the key it was encrypted with: %w", path, ErrCipherKeyRejected)
}

// hasPlainHeader reports whether path starts as an unencrypted SQLite
// file; an encrypted one starts with random bytes.
func hasPlainHeader(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	header := make([]byte, len(plainHeader))
	if _, err := io.ReadFull(file, header); err != nil {
		return false, err
	}
	return bytes.Equal(header, []byte(plainHeader)), nil
}
