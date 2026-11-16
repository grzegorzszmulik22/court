//go:build !windows

package commission

import (
	"os/exec"
	"syscall"
)

// detach starts the child in its own session so it outlives the
// commissioning terminal and is not taken down by a hangup or a group
// signal directed at `court commission`.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
