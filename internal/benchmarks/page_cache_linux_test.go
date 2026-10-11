package benchmarks

import (
	"errors"
	"os"
	"syscall"
)

// errPageCacheUnsupported is never returned on Linux.
var errPageCacheUnsupported = errors.New("dropping a file from the page cache is unsupported here")

// posixFadvDontNeed is POSIX_FADV_DONTNEED on Linux.
const posixFadvDontNeed = 4

func dropFromPageCache(file *os.File) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_FADVISE64, file.Fd(), 0, 0, posixFadvDontNeed, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
