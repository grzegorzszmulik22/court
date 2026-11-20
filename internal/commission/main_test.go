package commission_test

// TestMain builds the court binary once per test run when the caller did
// not: the check scripts under test/checks/ export COURT_BIN themselves,
// but plain `go test ./...` must work too. The build lands in a temp
// directory, court.exe on Windows (CR-001).
//
// It also owns the leak barrier the M2.1 chunk needed: every fixture
// records the pid of the daemon it started in a run-scoped registry, and
// after all tests have run TestMain fails when any recorded daemon is
// still alive. A test that asks a daemon to stop but does not wait for it
// is therefore a failing test, on Linux and Windows alike.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	registry, err := os.MkdirTemp("", "court-test-daemons-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "commission_test: daemon registry: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("COURT_TEST_DAEMON_REGISTRY", registry); err != nil {
		fmt.Fprintf(os.Stderr, "commission_test: daemon registry: %v\n", err)
		os.Exit(1)
	}
	code := runTests(m)
	code = reportLeakedDaemons(registry, code)
	if err := os.RemoveAll(registry); err != nil && code == 0 {
		fmt.Fprintf(os.Stderr, "commission_test: remove daemon registry: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

func runTests(m *testing.M) int {
	if os.Getenv("COURT_BIN") != "" {
		return m.Run()
	}
	bin, dir, err := buildCourt()
	if err != nil {
		// Tests that need the binary report the missing COURT_BIN
		// themselves; a build failure only warns here.
		fmt.Fprintf(os.Stderr, "commission_test: %v\n", err)
		return m.Run()
	}
	if err := os.Setenv("COURT_BIN", bin); err != nil {
		fmt.Fprintf(os.Stderr, "commission_test: COURT_BIN: %v\n", err)
		return 1
	}
	code := m.Run()
	// The test run is over; nothing this package started may still hold
	// the binary. RemoveAll failing is reported by reportLeakedDaemons
	// through the daemons it names, so the removal itself is best-effort.
	_ = os.RemoveAll(dir)
	return code
}

// recordDaemonPID writes pid into the run-scoped registry before a test
// asks the daemon to stop, so TestMain can prove after the run that it
// did stop. Writing is idempotent: a fixture shutdown may run twice.
func recordDaemonPID(t *testing.T, pid int) {
	t.Helper()
	dir := os.Getenv("COURT_TEST_DAEMON_REGISTRY")
	if dir == "" {
		t.Fatalf("COURT_TEST_DAEMON_REGISTRY is not set")
	}
	if pid <= 0 {
		t.Fatalf("cannot record daemon pid %d", pid)
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(pid)), nil, 0o644); err != nil {
		t.Fatalf("record daemon pid %d: %v", pid, err)
	}
}

// reportLeakedDaemons fails the run when a daemon recorded by a test is
// still alive after every test and its cleanup have finished.
func reportLeakedDaemons(registry string, code int) int {
	leaked, err := leakedDaemons(registry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "commission_test: %v\n", err)
		return 1
	}
	if len(leaked) > 0 {
		fmt.Fprintf(os.Stderr,
			"commission_test: %d court daemon process(es) from this run are still alive: %s\n",
			len(leaked), strings.Join(leaked, " "))
		return 1
	}
	return code
}

// leakedDaemons is the barrier's decision, separated so the self-test can
// assert it directly: every recorded pid that is still alive is a leak.
func leakedDaemons(registry string) ([]string, error) {
	entries, err := os.ReadDir(registry)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read daemon registry: %w", err)
	}
	var leaked []string
	for _, e := range entries {
		pid, err := strconv.Atoi(strings.TrimSpace(e.Name()))
		if err != nil {
			return nil, fmt.Errorf("bad daemon record %q: %w", e.Name(), err)
		}
		if processAlive(pid) {
			leaked = append(leaked, e.Name())
		}
	}
	return leaked, nil
}

// buildCourt compiles ./cmd/court from the repository root (two dirs up
// from this package) into a fresh temp dir. It returns the binary path
// and the temp dir to remove.
func buildCourt() (bin, dir string, err error) {
	dir, err = os.MkdirTemp("", "court-testbin-")
	if err != nil {
		return "", "", err
	}
	name := "court"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin = filepath.Join(dir, name)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", "", err
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/court")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("court build: %w\n%s", err, out)
	}
	return bin, dir, nil
}
