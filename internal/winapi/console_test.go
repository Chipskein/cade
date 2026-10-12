//go:build windows

package winapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnableVirtualTerminalRefusesAFile(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := EnableVirtualTerminal(file); err == nil {
		t.Fatal("expected an error for a file that is not a console")
	}
}
