// Package commission resolves the court commission command: repository
// detection, done-check detection, the built-in defaults table, the data
// directory keyed by repository path, and the flow that opens the
// commission, starts the orchestrator and, when stdin is a terminal,
// runs the upfront hearing of spec/milestones/M2 §2.5.
package commission

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// gitOut runs one git command in dir and returns combined output.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// RepoRoot returns the repository root containing dir, or an error when
// dir is not inside a git repository. Court commands refuse to run
// outside a repository with one sentence (C2.1.4).
func RepoRoot(dir string) (string, error) {
	out, err := gitOut(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", fmt.Errorf("not a git repository")
	}
	return root, nil
}

// Done-check detection rules, stated in the output when used (§ M2.1).
// One rule per ecosystem; every detected command becomes a done-check.
var doneCheckRules = []struct {
	name   string
	marker string // file at the repository root
	cmd    string
}{
	{"go", "go.mod", "go test ./..."},
	{"node", "package.json", "npm test"},
	{"python", "pyproject.toml", "python -m pytest"},
}

// DetectDoneChecks returns the test commands the repository layout makes
// detectable, in rule order, along with the rule names that fired.
// Neither means the list is empty and the caller must say so (C2.1.3).
func DetectDoneChecks(repo string) (cmds []string, rules []string) {
	for _, r := range doneCheckRules {
		if st, err := os.Stat(filepath.Join(repo, r.marker)); err == nil && !st.IsDir() {
			cmds = append(cmds, r.cmd)
			rules = append(rules, r.name)
		}
	}
	return cmds, rules
}

// Defaults is the built-in table the commission falls back to. Every
// value used because the human did not supply it is listed in
// commission.opened.defaults (C2.1.5).
var Defaults = struct {
	Tokens, Minutes, Speculation, Attention int64
	Territory                             []string
}{
	Tokens:      100000,
	Minutes:     60,
	Speculation: 2000,
	Attention:   30,
	Territory:   []string{"**"},
}

// stateRoot is the per-user state directory Court keeps its data under.
func stateRoot() string {
	if x := os.Getenv("COURT_STATE_DIR"); x != "" {
		return x
	}
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "court")
	}
	if runtime.GOOS == "windows" {
		if x := os.Getenv("LOCALAPPDATA"); x != "" {
			return filepath.Join(x, "court")
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".local", "state", "court")
	}
	return filepath.Join(os.TempDir(), "court-state")
}

// DataDir resolves the Court data directory for a repository: the
// explicit --data-dir flag wins, then the per-repo key under the user's
// state directory. The key mixes a readable repository base name with a
// hash of the absolute path so two checkouts stay separate.
func DataDir(flag, repo string) (string, error) {
	if flag != "" {
		abs, err := filepath.Abs(flag)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(abs))
	key := fmt.Sprintf("%s-%s", sanitizeKey(filepath.Base(abs)), hex.EncodeToString(sum[:8]))
	return filepath.Join(stateRoot(), key), nil
}

func sanitizeKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "repo"
	}
	return b.String()
}

// CommissionID mints a commission id of the nameRe shape: 'c' plus hex.
func CommissionID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "c" + hex.EncodeToString(b[:]), nil
}

// SortedCategories returns the closed category list in a stable order
// for output and fixtures.
func SortedCategories(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
