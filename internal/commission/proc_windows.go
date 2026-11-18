//go:build windows

package commission

import (
	"errors"
	"os/exec"

	"golang.org/x/sys/windows"
)

// detach is a no-op for process lifetime on Windows: a started child keeps
// running after the parent exits. It does stop the child from inheriting
// this process's standard handles. os/exec makes the handles it passes to
// the child inheritable and CreateProcess copies every inheritable handle,
// so without this a daemon or agent that outlives its test keeps the test's
// output pipe open ("Test I/O incomplete 12s after exiting").
func detach(cmd *exec.Cmd) error {
	for _, h := range []windows.Handle{windows.Stdin, windows.Stdout, windows.Stderr} {
		if h == 0 {
			continue
		}
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0); err != nil &&
			!errors.Is(err, windows.ERROR_INVALID_HANDLE) {
			return err
		}
	}
	return nil
}
