//go:build windows

package dbkey

import "github.com/chipskein/cade/internal/winapi"

// dpapiEntropy ties the sealed key to cade: DPAPI refuses it to a program
// that unseals it without these bytes.
var dpapiEntropy = []byte("cade-database")

// WindowsDPAPI is the BlobProtector of the current Windows account.
type WindowsDPAPI struct{}

func (WindowsDPAPI) Protect(data []byte) ([]byte, error) {
	return winapi.Protect(data, dpapiEntropy)
}

func (WindowsDPAPI) Unprotect(blob []byte) ([]byte, error) {
	return winapi.Unprotect(blob, dpapiEntropy)
}
