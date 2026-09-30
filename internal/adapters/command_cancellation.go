package adapters

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Bound foreground helpers and inherited output pipes, not systemd-owned jobs.
// CommandContext alone kills only the parent; a Git/Nix child can otherwise
// retain the pipe and prevent a cancelled read from ever returning.
func configureCommandCancellation(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
}
