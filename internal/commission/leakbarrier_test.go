package commission_test

// The leak barrier must test itself: a harness that never fails on a leak
// is indistinguishable from one with no leaks. This test deliberately
// leaves a real `court daemon` running in a private registry and requires
// the barrier to report it and to keep reporting it until the process is
// actually gone; after waitExit it requires the process itself to be dead,
// not just its record cleared. It runs on Windows and Linux, so a disabled
// barrier or a wait loop that returns without waiting turns this test (and
// therefore make regress) red.

import (
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func requestShutdown(t *testing.T, url, token string) int {
	t.Helper()
	req, err := http.NewRequest("POST", url+"/admin/shutdown", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("shutdown request: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatalf("shutdown answer: %v", err)
	}
	return resp.StatusCode
}

func TestLeakBarrierReportsLiveDaemon(t *testing.T) {
	reg := t.TempDir()
	t.Setenv("COURT_TEST_DAEMON_REGISTRY", reg)

	d := startTestDaemon(t, filepath.Join(t.TempDir(), "data"))
	url, token := d.waitReady(t)

	// 1. The barrier reports the live daemon.
	leaked, err := leakedDaemons(reg)
	if err != nil {
		t.Fatal(err)
	}
	want := strconv.Itoa(d.pid)
	if len(leaked) != 1 || leaked[0] != want {
		t.Fatalf("barrier reported %v, want the deliberately leaked daemon %s", leaked, want)
	}
	if code := reportLeakedDaemons(reg, 0); code != 1 {
		t.Fatalf("reportLeakedDaemons returned %d with a live daemon, want 1", code)
	}

	// 2. A wait loop that returns without waiting must not call a live
	// process gone, and must not clear its record.
	if err := waitProcessGoneErr(d.pid, 300*time.Millisecond); err == nil {
		t.Fatal("the barrier reported a live daemon gone")
	}
	if leaked, err := leakedDaemons(reg); err != nil || len(leaked) != 1 {
		t.Fatalf("the barrier cleared a live daemon's record: %v (%v)", leaked, err)
	}

	// 3. A recorded pid that cannot be alive is not a leak. MaxInt32 is
	// beyond every platform's pid range, unlike a small pid that a busy
	// host may hand to a real process.
	dead := strconv.Itoa(math.MaxInt32)
	if err := os.WriteFile(filepath.Join(reg, dead), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if leaked, err := leakedDaemons(reg); err != nil || len(leaked) != 1 {
		t.Fatalf("a max pid changed the leak set: %v (%v)", leaked, err)
	}

	// 4. A daemon asked to stop with a token it does not accept stays
	// alive; the barrier must report it rather than clear it.
	if code := requestShutdown(t, url, "wrong-token"); code != http.StatusUnauthorized {
		t.Fatalf("refused shutdown answered %d, want 401", code)
	}
	if err := waitProcessGoneErr(d.pid, 300*time.Millisecond); err == nil {
		t.Fatal("the barrier reported a daemon gone after a refused shutdown")
	}
	if !processAlive(d.pid) {
		t.Fatal("daemon died unexpectedly after a refused shutdown")
	}
	if leaked, err := leakedDaemons(reg); err != nil || len(leaked) != 1 {
		t.Fatalf("the barrier cleared a daemon that did not exit: %v (%v)", leaked, err)
	}

	// 5. Stop it for real. After waitExit returns, the process itself must
	// be gone (not only its registry record), and the barrier must be clean.
	if code := requestShutdown(t, url, token); code != http.StatusOK {
		t.Fatalf("shutdown answered %d, want 200", code)
	}
	if err := d.waitExit(t, 15*time.Second); err != nil {
		t.Fatalf("daemon exited with %v after /admin/shutdown", err)
	}
	if processAlive(d.pid) {
		t.Fatal("waitExit returned while the daemon was still alive")
	}
	if leaked, err := leakedDaemons(reg); err != nil || len(leaked) != 0 {
		t.Fatalf("barrier still reports %v after the daemon stopped (%v)", leaked, err)
	}
	if code := reportLeakedDaemons(reg, 0); code != 0 {
		t.Fatalf("reportLeakedDaemons returned %d with no leak, want 0", code)
	}
}
