//go:build windows

package winapi

import (
	"bytes"
	"testing"
)

func TestProtectRoundTripsAndHidesTheData(t *testing.T) {
	data, entropy := bytes.Repeat([]byte{0x11}, 32), []byte("cade-database")
	blob, err := Protect(data, entropy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, data) {
		t.Fatal("the protected blob holds the data in the clear")
	}
	plain, err := Unprotect(blob, entropy)
	if err != nil || !bytes.Equal(plain, data) {
		t.Errorf("unprotect = %x, %v; want the data back", plain, err)
	}
}

func TestUnprotectRefusesOtherEntropy(t *testing.T) {
	blob, err := Protect([]byte("secret"), []byte("cade-database"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Unprotect(blob, []byte("another purpose")); err == nil {
		t.Error("unprotect with other entropy succeeded, want a failure")
	}
}
