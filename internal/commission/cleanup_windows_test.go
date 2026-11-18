//go:build windows

package commission_test

// Windows process-exit barriers for the cleanup of a fixture's daemon.
// On Windows an open handle also blocks removal, so TempDir cleanup fails
// unless the daemon and the scripted agent have exited. Waiting until the
// process object is signalled (and the log files can be opened
// exclusively) is how the fixture knows both are gone — and a process
// that never exits fails the test instead of being papered over.

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// processIdentity returns the process object's creation time as a string,
// or "" when the process is gone. A pid alone is not an identity: the host
// is shared and Windows recycles pids, so a leaked-daemon report must not
// count an unrelated process that inherited the pid.
func processIdentity(pid int) string {
	if pid <= 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return ""
	}
	return strconv.FormatInt(creation.Nanoseconds(), 10)
}

// processMatches reports whether the live pid is still the process the
// registry recorded. An empty identity falls back to the pid alone.
func processMatches(pid int, ident string) bool {
	// Without a recorded identity there is no proof the live pid is ours:
	// report nothing rather than blame an unrelated process. A daemon that
	// really was left behind is caught by its fixture's waitProcessGone /
	// waitFileReleased barriers.
	if ident == "" {
		return false
	}
	return processIdentity(pid) == ident
}

// processAlive reports whether pid is a live process. OpenProcess fails
// for a pid whose process object is gone; an exited process that still
// has open handles is signalled immediately.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// A process object that is gone fails with an invalid parameter;
		// any other failure (access denied under security software) does
		// not prove the process is gone, so err on the side of alive.
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER) &&
			!errors.Is(err, windows.ERROR_NOT_FOUND)
	}
	defer windows.CloseHandle(h)
	event, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return true
	}
	return event != windows.WAIT_OBJECT_0
}

// waitProcessGone blocks until pid has exited, and fails the test when it
// is still running after timeout.
func waitProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("court daemon pid %d is still running after %s; "+
				"a process the test started was not shut down", pid, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitFileReleased waits until path can be opened exclusively, i.e. no
// process the test started still holds it.
func waitFileReleased(t *testing.T, path string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("probe %s: %v", path, err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil,
			windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			windows.CloseHandle(h)
			return
		}
		if err == windows.ERROR_FILE_NOT_FOUND || err == windows.ERROR_PATH_NOT_FOUND {
			return
		}
		if err != windows.ERROR_SHARING_VIOLATION && err != windows.ERROR_LOCK_VIOLATION {
			t.Fatalf("probe %s: %v", path, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s is still open 20s after the commission stopped; "+
				"a process a test started is still running", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
