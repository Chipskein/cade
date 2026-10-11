package console

import (
	"os"
	"path/filepath"
	"testing"
)

func TestARegularFileIsNoTerminal(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if ANSITerminal(file) {
		t.Fatal("expected a regular file not to be an ANSI terminal")
	}
}

func TestAPipeIsNoTerminal(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if ANSITerminal(writer) {
		t.Fatal("expected a pipe not to be an ANSI terminal")
	}
}
