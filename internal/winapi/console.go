//go:build windows

package winapi

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// EnableVirtualTerminal turns on VT processing for the console file
// writes to, so it draws ANSI sequences; Windows Terminal has it on,
// conhost does not. It fails when file is not a console.
//
//	err := winapi.EnableVirtualTerminal(os.Stderr)
func EnableVirtualTerminal(file *os.File) error {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return fmt.Errorf("read the console mode of %s: %w", file.Name(), err)
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return fmt.Errorf("turn on VT processing for %s: %w", file.Name(), err)
	}
	return nil
}
