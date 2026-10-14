package sqlitestore

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chipskein/cade/internal/testcheck"
)

// The driver is built with the SQLite3 Multiple Ciphers amalgamation (#111);
// a build that fell back to the plain SQLite would have no cipher to use.
func TestDriverIsSQLite3MultipleCiphers(t *testing.T) {
	db, err := sql.Open(DriverName, ":memory:")
	testcheck.NoError(t, err)
	defer db.Close()
	var version string
	testcheck.NoError(t, db.QueryRowContext(context.Background(), "SELECT sqlite3mc_version()").Scan(&version))
	if !strings.HasPrefix(version, "SQLite3 Multiple Ciphers") {
		t.Errorf("sqlite3mc_version() = %q, want SQLite3 Multiple Ciphers", version)
	}
}

// Without a key the driver must keep reading and writing plain SQLite
// files: browser history, msgstore.db and today's cade.db.
func TestPlainSQLiteFileOpensWithoutKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "History")
	db, err := sql.Open(DriverName, "file:"+path)
	testcheck.NoError(t, err)
	_, err = db.ExecContext(ctx, "CREATE TABLE urls (url TEXT); INSERT INTO urls VALUES ('https://example.com')")
	testcheck.NoError(t, err)
	testcheck.NoError(t, db.Close())
	header, err := os.ReadFile(path)
	testcheck.NoError(t, err)
	if !bytes.HasPrefix(header, []byte(plainHeader)) {
		t.Fatalf("file starts with %q, want the plain SQLite header", header[:len(plainHeader)])
	}
	reopened, err := sql.Open(DriverName, "file:"+path+"?mode=ro")
	testcheck.NoError(t, err)
	defer reopened.Close()
	var url string
	testcheck.NoError(t, reopened.QueryRowContext(ctx, "SELECT url FROM urls").Scan(&url))
	if url != "https://example.com" {
		t.Errorf("url = %q, want https://example.com", url)
	}
}
