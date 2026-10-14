//go:build windows

package winapi

import (
	"bytes"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapiFlags forbid any prompt: cade also runs from the scheduled ingest,
// with no desktop to show one on.
const dpapiFlags = windows.CRYPTPROTECT_UI_FORBIDDEN

// Protect encrypts data with DPAPI for the current Windows account: only
// that account, on this machine or where its profile roams, decrypts it.
// entropy must be given again to Unprotect.
//
//	blob, err := winapi.Protect(key, []byte("cade-database"))
func Protect(data, entropy []byte) ([]byte, error) {
	var protected windows.DataBlob
	if err := windows.CryptProtectData(blobOf(data), nil, blobOf(entropy), 0, nil, dpapiFlags, &protected); err != nil {
		return nil, fmt.Errorf("protect %d bytes with DPAPI: %w", len(data), err)
	}
	return takeBlob(&protected), nil
}

// Unprotect decrypts what Protect returned, with the same entropy.
//
//	key, err := winapi.Unprotect(blob, []byte("cade-database"))
func Unprotect(blob, entropy []byte) ([]byte, error) {
	var plain windows.DataBlob
	if err := windows.CryptUnprotectData(blobOf(blob), nil, blobOf(entropy), 0, nil, dpapiFlags, &plain); err != nil {
		return nil, fmt.Errorf("unprotect a %d-byte DPAPI blob: %w", len(blob), err)
	}
	return takeBlob(&plain), nil
}

func blobOf(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

// takeBlob copies a blob Windows allocated and frees it with LocalFree,
// as CryptProtectData's documentation requires.
func takeBlob(blob *windows.DataBlob) []byte {
	data := bytes.Clone(unsafe.Slice(blob.Data, blob.Size))
	// A failed free leaks a few bytes; there is nothing to do about it.
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(blob.Data)))
	return data
}
