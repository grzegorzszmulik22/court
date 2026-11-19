//go:build !windows

package world_test

// C1.4.1, Discard error reporting: the POSIX way to make the
// packed-refs removal fail is to take write permission off the
// repository's .git directory after `git branch -D` has succeeded. A
// shim on PATH does that at exactly the step Discard performs, so the
// worktree and branch removals succeed and only the residue removal
// fails. Linux keeps this form; Windows has its own in
// discard_err_windows_test.go.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func discardReportsPackedRefsError(t *testing.T, repo, gitDir string) error {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git on PATH")
	}

	shimDir := t.TempDir()
	shim := filepath.Join(shimDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
%q "$@"
rc=$?
if [ "$1" = branch ] && [ "$2" = -D ] && [ $rc -eq 0 ] && [ -f %q/packed-refs ]; then
	chmod 0555 %q
fi
exit $rc
`, realGit, gitDir, gitDir)
	if err := os.WriteFile(shim, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+":"+os.Getenv("PATH"))

	root := open(t, repo)
	w, err := root.Fork(head(t, repo), "x")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	defer os.Chmod(gitDir, 0755) // let t.TempDir clean up
	return w.Discard()
}
