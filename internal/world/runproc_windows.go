//go:build windows

package world

import (
	"os/exec"
	"strconv"
)

// procAttr is a no-op on Windows: the process tree is killed through
// taskkill's /T walk, which needs no spawn-time attributes.
func procAttr(cmd *exec.Cmd) {}

// killProcTree kills the command and every descendant it started:
// taskkill /T walks the process tree, /F forces. The process itself is
// killed as well so Run returns even if taskkill is unavailable.
func killProcTree(cmd *exec.Cmd) {
	// Best effort: taskkill fails when the tree already exited, and Kill then
	// covers the started process. C1.4.6 checks that no descendant survives.
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	_ = cmd.Process.Kill()
}
