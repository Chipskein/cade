//go:build !unix

package procctl

import "fmt"

func (System) Alive(int) bool { return false }

func (System) Terminate(pid int) error {
	return fmt.Errorf("stop process %d: %w", pid, ErrUnsupported)
}

func (System) Pause(pid int) error {
	return fmt.Errorf("pause process %d: %w", pid, ErrUnsupported)
}

func (System) Continue(pid int) error {
	return fmt.Errorf("continue process %d: %w", pid, ErrUnsupported)
}

func (System) StartDetached(command DetachedCommand) (int, error) {
	return 0, fmt.Errorf("start %v in the background: %w", command.Args, ErrUnsupported)
}

func (System) LowerPriority() error {
	return fmt.Errorf("lower priority: %w", ErrUnsupported)
}
