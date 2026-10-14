//go:build !windows

package dbkey

import (
	"errors"
	"runtime"
)

// errNoDPAPI: DPAPI is a Windows API; elsewhere the platform picks another
// vault, so reaching this is a wiring mistake.
var errNoDPAPI = errors.New("DPAPI exists only on Windows, not on " + runtime.GOOS)

// WindowsDPAPI is the BlobProtector of the current Windows account; on
// other systems every call fails.
type WindowsDPAPI struct{}

func (WindowsDPAPI) Protect([]byte) ([]byte, error) {
	return nil, errNoDPAPI
}

func (WindowsDPAPI) Unprotect([]byte) ([]byte, error) {
	return nil, errNoDPAPI
}
