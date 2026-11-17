//go:build !windows

package world_test

import (
	"os/exec"
	"syscall"
)

// detachHelper starts the heartbeat in its own session, like setsid: it
// leaves the process group the time limit can signal directly.
func detachHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
