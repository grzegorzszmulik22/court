//go:build windows

package commission

import "os/exec"

// detach is a no-op on Windows: a started child keeps running after the
// parent exits, and there is no controlling-terminal hangup to escape.
func detach(cmd *exec.Cmd) {}
