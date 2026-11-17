// Copyright 2026 The Court Authors
// SPDX-License-Identifier: Apache-2.0

package commission

import (
	"os"
	"strings"
	"testing"
)

// The hearing runs only when standard input is a terminal. A pipe never
// counts (C2.5.3); on Windows, Git Bash's mintty pty must count, so the
// check is console-or-Cygwin-pty, not console only.
func TestTerminalIn(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if terminalIn(r) {
		t.Fatal("a pipe was taken for a terminal")
	}
	if terminalIn(strings.NewReader("y\n")) {
		t.Fatal("a non-file reader was taken for a terminal")
	}

	// Each detector alone must be enough.
	defer func(old func(uintptr) bool) { isTerminalFd = old }(isTerminalFd)
	isTerminalFd = func(uintptr) bool { return true }
	if !terminalIn(r) {
		t.Fatal("a file the detector accepts was not taken for a terminal")
	}
}
