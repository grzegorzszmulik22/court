//go:build !windows

package commission_test

// Unix process-exit barriers for the cleanup of a fixture's daemon.
// Windows needs the same properties through its own handles; see
// cleanup_windows_test.go. Neither file starts or kills anything: a test
// only waits on a process it started itself.

import (
	"errors"
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

// waitFileReleased is a no-op on Unix: an open file does not block its
// removal, so there is no Windows-style cleanup barrier to wait for.
func waitFileReleased(t *testing.T, path string) {}

// processIdentity has no portable Unix form needed here; the leak barrier
// falls back to the pid alone.
func processIdentity(pid int) string { return "" }

// processMatches reports whether the live pid is still the recorded
// process. Without an identity the pid alone decides.
func processMatches(pid int, ident string) bool { return true }

// normalizePTYOutput is a no-op where the pty is a real one.
func normalizePTYOutput(s string) string { return s }
