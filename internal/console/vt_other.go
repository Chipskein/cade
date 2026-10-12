//go:build !windows

package console

import "os"

// enableANSI has nothing to do: unix terminals draw ANSI sequences.
func enableANSI(*os.File) bool { return true }
