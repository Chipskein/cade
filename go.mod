module github.com/chipskein/cade

go 1.27.1

require (
	github.com/andybalholm/brotli v1.2.6
	github.com/asg017/sqlite-vec-go-bindings v0.1.6
	github.com/klauspost/compress v1.20.1
	github.com/magefile/mage v1.17.2
	github.com/mattn/go-sqlite3 v1.14.52
	golang.org/x/image v0.46.0
	golang.org/x/sys v0.48.0
)

tool github.com/magefile/mage

// The driver with the SQLite3 Multiple Ciphers amalgamation (#111), built by
// any go tool mage target that compiles (internal/devtasks/sqlite3mc.go).
replace github.com/mattn/go-sqlite3 => ./third_party/go-sqlite3mc
