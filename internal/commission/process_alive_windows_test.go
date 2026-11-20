//go:build windows

package commission_test

// The two Windows fixes need tests that fail when reverted:
//   - processAlive must treat an OpenProcess failure other than "gone" as
//     alive (here: access denied, injected through the openProcess seam);
//   - detach must clear standard-handle inheritance (see the internal
//     detach_windows_test.go, which probes the child's handle table).

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProcessAliveAccessDeniedIsAlive(t *testing.T) {
	old := openProcess
	defer func() { openProcess = old }()

	openProcess = func(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error) {
		return 0, windows.ERROR_ACCESS_DENIED
	}
	if !processAlive(4242) {
		t.Fatal("a process OpenProcess cannot open (access denied) was reported gone")
	}

	openProcess = func(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error) {
		return 0, windows.ERROR_INVALID_PARAMETER
	}
	if processAlive(4242) {
		t.Fatal("a process that does not exist (invalid parameter) was reported alive")
	}

	openProcess = func(desiredAccess uint32, inheritHandle bool, processID uint32) (windows.Handle, error) {
		return 0, windows.ERROR_NOT_FOUND
	}
	if processAlive(4242) {
		t.Fatal("a process that does not exist (not found) was reported alive")
	}

	openProcess = old
	if !processAlive(os.Getpid()) {
		t.Fatal("the test's own live process was reported dead")
	}
}
