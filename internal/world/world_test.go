package world_test

// Helpers shared by the C1.4 check tests. Test repositories are created in
// temporary directories that Go removes; the lane never forks or configures a
// repository it did not create (tools/host.md, lane note).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"court/internal/world"
)

// helperArgv re-executes the test binary itself as a portable subprocess for
// Run's timeout, tree-kill and stream checks — no shell and no sleep binary,
// so the checks run identically on Windows and Linux (CR-001).
func helperArgv(mode string) []string {
	return []string{os.Args[0], "-test.run=^TestHelperProcess$", "--", mode}
}

// TestHelperProcess is not a test; it is the subprocess helperArgv starts.
// COURT_TEST_HELPER gates it so an ordinary `go test` run just returns.
// Modes: "sleep" (sleep 30s), "tree" (spawn a sleeping child holding this
// process's output pipes, then sleep), "echo" (write to stdout and stderr),
// or a decimal exit code.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("COURT_TEST_HELPER") != "1" {
		return
	}
	// write <path> <content> and delete <path>: a world-run process that
	// changes a file outside its own worktree.
	if len(os.Args) >= 3 && os.Args[len(os.Args)-3] == "write" {
		if err := os.WriteFile(os.Args[len(os.Args)-2],
			[]byte(os.Args[len(os.Args)-1]+"\n"), 0o644); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if len(os.Args) >= 2 && os.Args[len(os.Args)-2] == "delete" {
		if err := os.Remove(os.Args[len(os.Args)-1]); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	// detach <file>: start a heartbeat in a detached session/process
	// and exit at once, the way setsid/nohup or `start /b` leaves a
	// descendant behind.
	if len(os.Args) >= 2 && os.Args[len(os.Args)-2] == "detach" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$",
			"--", "heartbeat", os.Args[len(os.Args)-1])
		cmd.Env = os.Environ()
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		detachHelper(cmd)
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	// heartbeat <file>: append one byte every 50 ms for 10 s. The
	// detached-Run check uses it as a descendant that would keep
	// working if the time limit failed to kill the tree.
	if len(os.Args) >= 2 && os.Args[len(os.Args)-2] == "heartbeat" {
		f, err := os.OpenFile(os.Args[len(os.Args)-1],
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			os.Exit(2)
		}
		for i := 0; i < 200; i++ {
			f.WriteString("x")
			time.Sleep(50 * time.Millisecond)
		}
		f.Close()
		os.Exit(0)
	}
	switch mode := os.Args[len(os.Args)-1]; mode {
	case "sleep":
		time.Sleep(30 * time.Second)
	case "tree":
		// A descendant holding this process's stdout/stderr: if the timeout
		// kills only the parent, the pipes stay open and Run hangs until the
		// child's 30s are up — which is how the checks detect tree-kill.
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$", "--", "sleep")
		cmd.Env = os.Environ()
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(30 * time.Second)
	case "echo":
		fmt.Println("out")
		fmt.Fprintln(os.Stderr, "err")
	default:
		code, err := strconv.Atoi(mode)
		if err != nil {
			os.Exit(2)
		}
		os.Exit(code)
	}
	os.Exit(0)
}

// git runs git in dir with a clean git environment (no inherited GIT_*
// variables, so what a test measures is file config, never env config).
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitE(dir, args...)
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return out
}

// gitE is git but returns (output, error).
func gitE(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = cleanGitEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func cleanGitEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		k := e[:strings.IndexByte(e, '=')]
		if k == "GIT_DIR" || k == "GIT_WORK_TREE" || strings.HasPrefix(k, "GIT_CONFIG") {
			continue
		}
		env = append(env, e)
	}
	return env
}

// newRepo creates a plain test repository with nCommits commits and returns
// its top-level path.
func newRepo(t *testing.T, nCommits int) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.email", "court@test")
	git(t, dir, "config", "user.name", "court")
	git(t, dir, "config", "commit.gpgsign", "false")
	for i := 0; i < nCommits; i++ {
		writeFile(t, dir, "file"+strconv.Itoa(i)+".txt", "commit "+strconv.Itoa(i)+"\n")
		git(t, dir, "add", "-A", ".")
		git(t, dir, "commit", "-qm", "c"+strconv.Itoa(i))
	}
	return dir
}

// open opens the repository under a fresh data directory.
func open(t *testing.T, repo string) *world.GitWorld {
	t.Helper()
	data := filepath.Join(t.TempDir(), "data")
	w, err := world.Open(repo, data, "Ctest")
	if err != nil {
		t.Fatalf("world.Open: %v", err)
	}
	return w
}

func head(t *testing.T, dir string) world.Commit {
	return world.Commit(strings.TrimSpace(git(t, dir, "rev-parse", "HEAD")))
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
