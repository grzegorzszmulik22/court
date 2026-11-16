//go:build !windows

package world

import (
	"os/exec"
	"syscall"
)

// procAttr puts the command in its own process group so a timeout can kill
// the command and every descendant it spawned with one signal.
func procAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcTree kills the process group the command leads: the command and
// every descendant still in the group die together, so a child holding the
// output pipes cannot hang Run after its parent is gone.
func killProcTree(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
