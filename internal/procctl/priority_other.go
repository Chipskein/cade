//go:build unix && !linux

package procctl

import (
	"fmt"
	"syscall"
)

// lowestNice is the lowest scheduling priority.
const lowestNice = 19

// LowerPriority renices the process; elsewhere than Linux, nice applies
// to the whole process and there is no portable I/O class.
func (System) LowerPriority() error {
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, lowestNice); err != nil {
		return fmt.Errorf("set nice %d: %w", lowestNice, err)
	}
	return nil
}
