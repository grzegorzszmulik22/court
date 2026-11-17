//go:build !windows

package commission_test

// The C2.5.x hearing checks run `court commission` under util-linux
// `script`, which gives it a real pty on the standard input. Windows
// (no `script`) has its own ConPTY implementation in pty_windows_test.go.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// commissionPTY runs `court commission` with stdin on a pty fed the
// answer lines in answers.
func (f *fixture) commissionPTY(answers string, args ...string) (string, error) {
	f.t.Helper()
	argv := append([]string{"commission"}, args...)
	for i, a := range argv {
		argv[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	line := fmt.Sprintf("cd %s && exec %s %s", f.repo, f.bin,
		strings.Join(argv, " "))
	cmd := exec.Command("script", "-qec", line, "-E", "never", os.DevNull)
	cmd.Stdin = strings.NewReader(answers)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
