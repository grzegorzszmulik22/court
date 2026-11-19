//go:build windows

package world_test

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachHelper starts the heartbeat in a new process group with no
// console: the detached shape B3-07 is about.
func detachHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
}
