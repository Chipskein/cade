//go:build !linux

package benchmarks

import (
	"errors"
	"os"
)

// errPageCacheUnsupported makes the cold-cache benchmarks skip outside
// Linux, where fadvise is not reachable from Go's syscall package (#101).
var errPageCacheUnsupported = errors.New("dropping a file from the page cache is unsupported here")

func dropFromPageCache(*os.File) error {
	return errPageCacheUnsupported
}
