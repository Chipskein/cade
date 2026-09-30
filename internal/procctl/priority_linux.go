package procctl

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

// Lowest scheduling priority, and the idle I/O class of ioprio_set(2):
// the process only reads the disk when nothing else does.
const (
	lowestNice         = 19
	ioprioWhoProcess   = 1
	ioprioClassIdle    = 3
	ioprioClassShift   = 13
	threadListLocation = "/proc/self/task"
)

// LowerPriority renices every thread: on Linux, nice and the I/O class
// belong to a thread, and threads started later inherit them from the
// thread that starts them, so all existing ones must change.
func (System) LowerPriority() error {
	threads, err := os.ReadDir(threadListLocation)
	if err != nil {
		return fmt.Errorf("list threads in %s: %w", threadListLocation, err)
	}
	for _, thread := range threads {
		tid, err := strconv.Atoi(thread.Name())
		if err != nil {
			continue
		}
		if err := lowerThreadPriority(tid); err != nil {
			return err
		}
	}
	return nil
}

func lowerThreadPriority(tid int) error {
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, tid, lowestNice); err != nil {
		return fmt.Errorf("set nice %d on thread %d: %w", lowestNice, tid, err)
	}
	idle := ioprioClassIdle << ioprioClassShift
	if _, _, errno := syscall.Syscall(syscall.SYS_IOPRIO_SET, ioprioWhoProcess, uintptr(tid), uintptr(idle)); errno != 0 {
		return fmt.Errorf("set idle I/O class on thread %d: %w", tid, errno)
	}
	return nil
}
