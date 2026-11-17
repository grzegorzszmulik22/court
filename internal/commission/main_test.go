package commission_test

// TestMain builds the court binary once per test run when the caller did
// not: the check scripts under test/checks/ export COURT_BIN themselves,
// but plain `go test ./...` must work too. The build lands in a temp
// directory, court.exe on Windows (CR-001).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("COURT_BIN") != "" {
		os.Exit(m.Run())
	}
	bin, dir, err := buildCourt()
	if err != nil {
		// Tests that need the binary report the missing COURT_BIN
		// themselves; a build failure only warns here.
		fmt.Fprintf(os.Stderr, "commission_test: %v\n", err)
		os.Exit(m.Run())
	}
	os.Setenv("COURT_BIN", bin)
	code := m.Run()
	// A daemon still running keeps its binary open on Windows; either way
	// the temp dir is expendable.
	_ = os.RemoveAll(dir)
	os.Exit(code)
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
