//go:build !windows

package commission_test

// Unix process-exit barriers for the cleanup of a fixture's daemon.
// Windows needs the same properties through its own handles; see
// cleanup_windows_test.go. Neither file starts or kills anything: a test
// only waits on a process it started itself.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// processAlive reports whether pid is a live process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// waitProcessGone blocks until pid has exited, and fails the test when it
// is still running after timeout. It is the cross-platform half of the
// fixture cleanup: asking a daemon to stop is not enough, the test must
// also see it gone (M2.1 leaked 15-21 daemons per run on Linux before
// this barrier existed).
// processGoneWithin waits up to timeout for pid to exit and reports
// whether it is gone. It never clears the registry: a record is removed
// only after the process is proven dead, and a live daemon must stay
// reported.
func processGoneWithin(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// waitProcessGoneErr is the barrier's decision: it waits for the process
// and clears the pid only when it is gone. A process that does not exit is
// reported and its record is kept.
func waitProcessGoneErr(pid int, timeout time.Duration) error {
	if !processGoneWithin(pid, timeout) {
		return fmt.Errorf("court daemon pid %d is still running after %s; "+
			"a process the test started was not shut down", pid, timeout)
	}
	clearDaemonPID(pid)
	return nil
}

// waitProcessGone fails the test when the process does not exit.
func waitProcessGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	if err := waitProcessGoneErr(pid, timeout); err != nil {
		t.Fatal(err)
	}
}

// waitFileReleased is a no-op on Unix: an open file does not block its
// removal, so there is no Windows-style cleanup barrier to wait for.
func waitFileReleased(t *testing.T, path string) {}

// normalizePTYOutput is a no-op where the pty is a real one.
func normalizePTYOutput(s string) string { return s }

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
