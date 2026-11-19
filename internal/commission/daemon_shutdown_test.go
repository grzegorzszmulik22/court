package commission_test

// Regression tests for the daemon process lifecycle (found on Windows,
// CR-002, then on Linux where M2.1 tests leaked 15-21 daemons per run):
// /admin/shutdown said the daemon stopped but `court daemon` kept running,
// and a daemon whose data directory was removed could never be found again
// (daemon.json, its only discovery record, was gone) yet ran forever.
//
// Both properties are platform-independent; Windows only makes the first
// visible at cleanup time, because an open log file blocks TempDir
// removal. The second is the product's own bound: see watchDaemonFile in
// internal/daemon/daemon.go (5 s of continuous absence, checked every
// 250 ms).

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testDaemon is one `court daemon` process a test started. Its cleanup
// kills the process only when a test failed before it exited; a passing
// test has already waited for the natural exit.
type testDaemon struct {
	cmd     *exec.Cmd
	done    chan error
	log     *os.File
	pid     int
	dataDir string
	exited  bool
}

func startTestDaemon(t *testing.T, dataDir string) *testDaemon {
	t.Helper()
	bin := os.Getenv("COURT_BIN")
	if bin == "" {
		t.Fatal("COURT_BIN is not set and TestMain could not build ./cmd/court")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dataDir, "daemon.log")
	logf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "daemon",
		"--data-dir", dataDir, "--ledger", filepath.Join(dataDir, "ledger.db"))
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		logf.Close()
		t.Fatal(err)
	}
	d := &testDaemon{cmd: cmd, done: make(chan error, 1), log: logf,
		pid: cmd.Process.Pid, dataDir: dataDir}
	go func() { d.done <- cmd.Wait() }()
	t.Cleanup(func() {
		if !d.exited {
			// The test started this process, so stopping it is cleanup,
			// not interference with someone else's work.
			_ = d.cmd.Process.Kill()
			<-d.done
			d.exited = true
		}
		d.log.Close()
	})
	return d
}

// waitReady waits for daemon.json and returns its URL and token.
func (d *testDaemon) waitReady(t *testing.T) (url, token string) {
	t.Helper()
	var df struct {
		URL   string `json:"url"`
		Token string `json:"admin_token"`
		PID   int    `json:"pid"`
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		raw, err := os.ReadFile(filepath.Join(d.dataDir, "daemon.json"))
		if err == nil {
			if err := json.Unmarshal(raw, &df); err != nil {
				t.Fatalf("daemon.json: %v", err)
			}
			if df.URL != "" {
				break
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("read daemon.json: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon did not write daemon.json in %s", d.dataDir)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if df.PID != d.pid {
		t.Fatalf("daemon.json pid = %d, process pid = %d", df.PID, d.pid)
	}
	recordDaemonPID(t, d.pid)
	return df.URL, df.Token
}

// waitExit waits for the process to end and fails the test when it is
// still running after timeout.
func (d *testDaemon) waitExit(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-d.done:
		d.exited = true
		clearDaemonPID(d.pid)
		return err
	case <-time.After(timeout):
		t.Fatalf("court daemon (pid %d) is still running %s after it should have exited",
			d.pid, timeout)
		return nil
	}
}

func TestDaemonExitsOnAdminShutdown(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	d := startTestDaemon(t, data)
	url, token := d.waitReady(t)

	req, _ := http.NewRequest("POST", url+"/admin/shutdown", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("shutdown request: %v", err)
	}
	resp.Body.Close()

	if err := d.waitExit(t, 15*time.Second); err != nil {
		t.Fatalf("daemon exited with %v after /admin/shutdown", err)
	}
	// The log must no longer be held by the process the test started.
	logPath := filepath.Join(data, "daemon.log")
	d.log.Close()
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("daemon.log still held after the daemon exited: %v", err)
	}
}

func TestDaemonExitsWhenDataDirRemoved(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	d := startTestDaemon(t, data)
	d.waitReady(t)

	// On Unix the whole data directory can be unlinked while the daemon
	// runs; on Windows the open ledger and log prevent that, so the file
	// that matters for discovery — daemon.json — is what disappears. Both
	// forms make the daemon unfindable, which is the trigger under test.
	if runtime.GOOS == "windows" {
		if err := os.Remove(filepath.Join(data, "daemon.json")); err != nil {
			t.Fatalf("remove daemon.json: %v", err)
		}
	} else if err := os.RemoveAll(data); err != nil {
		t.Fatalf("remove data dir: %v", err)
	}

	// The documented bound is 5 s (daemon.go); 15 s leaves room for a
	// loaded machine without weakening the property.
	if err := d.waitExit(t, 15*time.Second); err != nil {
		t.Fatalf("daemon exited with %v after its data directory was removed", err)
	}
}
