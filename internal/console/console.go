// Package console tells whether output goes to a terminal that draws ANSI
// sequences, which the progress bar and the status line redraw with.
package console

import "os"

// ANSITerminal reports whether file is a terminal that understands ANSI
// sequences, so the CLI may redraw a progress line in place. On Windows it
// first turns on the console's VT processing, which conhost leaves off; a
// console that refuses it gets the plain output (#54).
//
//	interactive := console.ANSITerminal(os.Stderr)
func ANSITerminal(file *os.File) bool {
	return isCharDevice(file) && enableANSI(file)
}

// isCharDevice reports whether file is a character device, a TTY or a
// console, rather than a file or a pipe.
func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
