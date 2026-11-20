package main

// Applying scenario scripts to repository fixtures: copy the fixture,
// replay every agent's edit steps (unified-diff patches or full writes)
// into that agent's own tree — each scripted agent works in its own
// world, which for validation is a separate copy of the base repository —
// then run the commission's done-checks and take `git diff --stat`
// against the base, the same diff summary a human would see.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// applyAgentEdits replays agent's edit steps into dir (a copy of repo/).
// Returns the sorted list of changed paths.
func applyAgentEdits(dir string, a *scriptAgent) ([]string, error) {
	changed := map[string]bool{}
	for _, st := range a.Steps {
		if st.Kind != "edit" || st.Edit == nil {
			continue
		}
		p := filepath.FromSlash(st.Edit.Path)
		abs := filepath.Join(dir, p)
		if st.Edit.Delete {
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			changed[st.Edit.Path] = true
			continue
		}
		if st.Edit.Write != "" {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(abs, []byte(st.Edit.Write), 0o644); err != nil {
				return nil, err
			}
			changed[st.Edit.Path] = true
			continue
		}
		raw, err := os.ReadFile(abs)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		out, err := applyPatch(string(raw), st.Edit.Patch)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", st.Edit.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(abs, []byte(out), 0o644); err != nil {
			return nil, err
		}
		changed[st.Edit.Path] = true
	}
	var paths []string
	for p := range changed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// agentEdits returns the edit steps of one agent in order.
func agentEdits(a *scriptAgent) []*editStep {
	var out []*editStep
	for _, st := range a.Steps {
		if st.Kind == "edit" && st.Edit != nil {
			out = append(out, st.Edit)
		}
	}
	return out
}

// applyPatch applies one unified diff (headers + @@ hunks) to before.
// Same contract as the scripted agent's applier: context is exact.
func applyPatch(before, patch string) (string, error) {
	hadNL := before == "" || strings.HasSuffix(before, "\n")
	src := splitLines(before)
	var out []string
	si := 0
	lines := strings.Split(patch, "\n")
	i := 0
	for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
		i++
	}
	if i >= len(lines) {
		return "", fmt.Errorf("patch has no hunks")
	}
	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "@@ ") {
			i++
			continue
		}
		start, err := hunkStart(lines[i])
		if err != nil {
			return "", err
		}
		i++
		target := start - 1
		if target < 0 {
			target = 0
		}
		for si < target && si < len(src) {
			out = append(out, src[si])
			si++
		}
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
			l := lines[i]
			if l == `\ No newline at end of file` {
				i++
				continue
			}
			switch {
			case strings.HasPrefix(l, " "):
				if si >= len(src) || src[si] != l[1:] {
					return "", fmt.Errorf("context mismatch line %d: want %q", si+1, l[1:])
				}
				out = append(out, l[1:])
				si++
			case strings.HasPrefix(l, "-"):
				if si >= len(src) || src[si] != l[1:] {
					return "", fmt.Errorf("remove mismatch line %d: want %q", si+1, l[1:])
				}
				si++
			case strings.HasPrefix(l, "+"):
				out = append(out, l[1:])
			case l == "" && si < len(src) && src[si] == "":
				out = append(out, "")
				si++
			case l == "":
				// trailing empty line of the patch text
			default:
				return "", fmt.Errorf("bad patch line %q", l)
			}
			i++
		}
	}
	for si < len(src) {
		out = append(out, src[si])
		si++
	}
	res := strings.Join(out, "\n")
	if hadNL && res != "" {
		res += "\n"
	}
	return res, nil
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func hunkStart(h string) (int, error) {
	end := strings.Index(h[3:], "@@")
	if end < 0 {
		return 0, fmt.Errorf("bad hunk header %q", h)
	}
	for _, f := range strings.Fields(h[3 : 3+end]) {
		if strings.HasPrefix(f, "-") {
			a, _, _ := strings.Cut(f[1:], ",")
			n, err := strconv.Atoi(a)
			if err != nil {
				return 0, fmt.Errorf("bad hunk header %q", h)
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("bad hunk header %q", h)
}

// ---------------------------------------------------------------------------
// tree helpers
// ---------------------------------------------------------------------------

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(to, raw, info.Mode().Perm())
	})
}

// treeFiles returns the regular files under root, slash-separated rel paths.
func treeFiles(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = raw
		return nil
	})
	return out, err
}

func treesEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || string(va) != string(vb) {
			return false
		}
	}
	return true
}

// gitDiffStat returns `git diff --stat` of the applied tree vs the fixture
// base: the diff summary a human would see. base must be a git commit in dir.
func gitDiffStat(dir string) (string, error) {
	if err := runGit(dir, "add", "-A"); err != nil {
		return "", err
	}
	out, err := exec.Command("git", "-C", dir, "diff", "--cached", "--stat").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff --stat: %w: %s", err, out)
	}
	return string(out), nil
}

func gitInitBase(dir string) error {
	if err := runGit(dir, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := runGit(dir, "add", "-A"); err != nil {
		return err
	}
	return runGit(dir, "-c", "user.email=court@local", "-c", "user.name=court",
		"commit", "-qm", "base")
}

func runGit(dir string, args ...string) error {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
