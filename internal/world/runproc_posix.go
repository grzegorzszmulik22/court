//go:build !windows

package world

// The time limit kills the command and every descendant, including one
// that detached with setsid or a double fork (red-team B3-07). The process
// group catches the ordinary tree; the per-run environment marker catches
// descendants that left it, by reading /proc.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func procAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

type procTree struct {
	cmd   *exec.Cmd
	token string
}

func startProc(cmd *exec.Cmd) (*procTree, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &procTree{cmd: cmd}, nil
}

// kill kills the process group and every process carrying this run's
// marker.
func (p *procTree) kill() {
	// Best effort: ESRCH means the group already exited.
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	p.killToken()
}

// close kills descendants the direct command left behind, even after a
// successful exit, so none outlives Run.
func (p *procTree) close() { p.kill() }

// killToken finds processes that inherited this run's marker in their
// environment and kills them. The marker is unique to this Run and is not
// in Court's own environment, so no unrelated process matches.
func (p *procTree) killToken() {
	if p.token == "" {
		return
	}
	marker := []byte(runTokenEnv + "=" + p.token)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	self := os.Getpid()
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err != nil || !bytesContains(raw, marker) {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func bytesContains(hay, needle []byte) bool {
	return len(needle) > 0 && strings.Contains(string(hay), string(needle))
}
