//go:build darwin

package procctl

// #include <sys/resource.h>
import "C"

import (
	"fmt"
	"syscall"
)

// lowestNice is the lowest scheduling priority.
const lowestNice = 19

// LowerPriority renices the process and throttles its disk I/O, macOS's
// counterpart of Linux's idle I/O class (#53): the ingestion then yields
// the disk to whatever the user is doing.
func (System) LowerPriority() error {
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, lowestNice); err != nil {
		return fmt.Errorf("set nice %d: %w", lowestNice, err)
	}
	if C.setiopolicy_np(C.IOPOL_TYPE_DISK, C.IOPOL_SCOPE_PROCESS, C.IOPOL_THROTTLE) != 0 {
		return fmt.Errorf("throttle disk I/O: setiopolicy_np failed")
	}
	return nil
}

// diskIOThrottled reports whether this process's disk I/O is throttled.
func diskIOThrottled() bool {
	return C.getiopolicy_np(C.IOPOL_TYPE_DISK, C.IOPOL_SCOPE_PROCESS) == C.IOPOL_THROTTLE
}
