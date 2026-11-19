//go:build windows

package world_test

// C1.4.1, Discard error reporting, Windows form. The POSIX test makes
// the packed-refs removal fail with directory permissions; Windows
// deletion ignores the read-only attribute and directory ACLs are not
// available to an unprivileged test, so the Windows way is to hold the
// file open without FILE_SHARE_DELETE: CreateFile with only READ|WRITE
// sharing makes the product's os.Remove fail with a sharing violation.
//
// A git shim on PATH runs `git branch -D` for real, then writes the
// header-only packed-refs file a git version can leave behind and waits
// until the test has taken its handle. Discard therefore reaches the
// residue removal with the file present and unremovable, and must
// report that failure instead of claiming success.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestGitShim is not a test: it is the git shim the Windows
// Discard-error test installs on PATH. COURT_GIT_SHIM gates it so an
// ordinary `go test` run only returns.
func TestGitShim(t *testing.T) {
	if os.Getenv("COURT_GIT_SHIM") != "1" {
		return
	}
	args := shimArgs(os.Args)
	cmd := exec.Command(os.Getenv("COURT_SHIM_REAL_GIT"), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	if code == 0 && len(args) >= 2 && args[0] == "branch" && args[1] == "-D" {
		// The residue a git version can leave where none existed before.
		header := "# pack-refs with: peeled fully-peeled sorted\n"
		if err := os.WriteFile(os.Getenv("COURT_SHIM_PACKED"), []byte(header), 0o644); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(os.Getenv("COURT_SHIM_READY"), []byte("1"), 0o644); err != nil {
			os.Exit(2)
		}
		// Hold the git invocation open until the test holds the file.
		deadline := time.Now().Add(15 * time.Second)
		release := os.Getenv("COURT_SHIM_RELEASE")
		for {
			if _, err := os.Stat(release); err == nil {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	os.Exit(code)
}

// shimArgs returns the arguments after the test binary's `--` separator.
func shimArgs(argv []string) []string {
	for i, a := range argv {
		if a == "--" {
			return argv[i+1:]
		}
	}
	return nil
}

func discardReportsPackedRefsError(t *testing.T, repo, gitDir string) error {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git on PATH")
	}

	root := open(t, repo)
	w, err := root.Fork(head(t, repo), "x")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}

	shimDir := t.TempDir()
	ready := filepath.Join(shimDir, "ready")
	release := filepath.Join(shimDir, "release")
	packed := filepath.Join(gitDir, "packed-refs")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("@echo off\r\n"+
		"\"%s\" -test.run=TestGitShim -- %%*\r\n"+
		"exit /b %%errorlevel%%\r\n", self)
	if err := os.WriteFile(filepath.Join(shimDir, "git.cmd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COURT_GIT_SHIM", "1")
	t.Setenv("COURT_SHIM_REAL_GIT", realGit)
	t.Setenv("COURT_SHIM_PACKED", packed)
	t.Setenv("COURT_SHIM_READY", ready)
	t.Setenv("COURT_SHIM_RELEASE", release)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Discard runs concurrently because its own git call (the shim)
	// waits for this test to take the handle.
	errCh := make(chan error, 1)
	go func() { errCh <- w.Discard() }()
	var h windows.Handle
	released := false
	t.Cleanup(func() {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if !released {
			_ = os.WriteFile(release, []byte("go"), 0o644)
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-errCh:
			t.Fatalf("Discard returned before branch -D: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the git shim did not reach branch -D")
		}
		time.Sleep(5 * time.Millisecond)
	}
	p, err := windows.UTF16PtrFromString(packed)
	if err != nil {
		t.Fatal(err)
	}
	h, err = windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("hold packed-refs open: %v", err)
	}
	released = true
	if err := os.WriteFile(release, []byte("go"), 0o644); err != nil {
		t.Fatal(err)
	}
	derr := <-errCh
	if h != 0 {
		windows.CloseHandle(h)
		h = 0
	}
	return derr
}
