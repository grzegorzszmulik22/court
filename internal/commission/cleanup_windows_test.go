//go:build windows

package commission_test

// Windows process-exit barriers for the cleanup of a fixture's daemon.
// On Windows an open handle also blocks removal, so TempDir cleanup fails
// unless the daemon and the scripted agent have exited. Waiting until the
// process object is signalled (and the log files can be opened
// exclusively) is how the fixture knows both are gone — and a process
// that never exits fails the test instead of being papered over.

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// processAlive reports whether pid is a live process. OpenProcess fails
// for a pid whose process object is gone; an exited process that still
// has open handles is signalled immediately.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
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
	clearDaemonPID(pid)
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

// clearDaemonPID removes a pid the test proved dead. A recorded pid whose
// process has exited can be reused by an unrelated process (a bash
// worker, another test's helper); keeping it would report a false leak.
func clearDaemonPID(pid int) {
	if pid <= 0 {
		return
	}
	dir := os.Getenv("COURT_TEST_DAEMON_REGISTRY")
	if dir == "" {
		return
	}
	_ = os.Remove(filepath.Join(dir, strconv.Itoa(pid)))
}
