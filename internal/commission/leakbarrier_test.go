package commission_test

// The leak barrier must test itself: a harness that never fails on a leak
// is indistinguishable from one with no leaks. This test deliberately
// leaves a real `court daemon` running in a private registry and requires
// the barrier to report it, then stops the daemon and requires the barrier
// to clear. It runs on Windows and Linux, so a disabled barrier on either
// OS turns this test (and therefore make regress) red.

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLeakBarrierReportsLiveDaemon(t *testing.T) {
	reg := t.TempDir()
	t.Setenv("COURT_TEST_DAEMON_REGISTRY", reg)

	d := startTestDaemon(t, filepath.Join(t.TempDir(), "data"))
	url, token := d.waitReady(t)

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

	// A recorded pid whose process does not exist is not a leak.
	if err := os.WriteFile(filepath.Join(reg, "999999"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if leaked, err := leakedDaemons(reg); err != nil || len(leaked) != 1 {
		t.Fatalf("dead pid changed the leak set: %v (%v)", leaked, err)
	}

	req, err := http.NewRequest("POST", url+"/admin/shutdown", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("shutdown request: %v", err)
	}
	resp.Body.Close()
	if err := d.waitExit(t, 15*time.Second); err != nil {
		t.Fatalf("daemon exited with %v after /admin/shutdown", err)
	}

	// waitExit proves the process gone and clears its record.
	leaked, err = leakedDaemons(reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaked) != 0 {
		t.Fatalf("barrier still reports %v after the daemon stopped", leaked)
	}
	if code := reportLeakedDaemons(reg, 0); code != 0 {
		t.Fatalf("reportLeakedDaemons returned %d with no leak, want 0", code)
	}
}
