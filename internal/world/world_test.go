package world_test

// Helpers shared by the C1.4 check tests. Test repositories are created in
// temporary directories that Go removes; the lane never forks or configures a
// repository it did not create (tools/host.md, lane note).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"court/internal/world"
)

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
