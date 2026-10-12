package console

import (
	"os"

	"github.com/chipskein/cade/internal/winapi"
)

// enableANSI turns on the console's VT processing (#54).
func enableANSI(file *os.File) bool {
	return winapi.EnableVirtualTerminal(file) == nil
}
