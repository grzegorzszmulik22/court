package world

// Shared-repository state that a world can change and Discard must restore
// (red-team B3-04, B3-05, B3-06; spec/02 §3 "restores the repository bit for
// bit", H3). Worktrees share refs, configuration, hooks, the main index and
// the main working tree. Fork records the state before its own worktree
// exists; Discard puts every one of them back before returning, so a world
// that ran `git tag`, `git config --local`, wrote a hook or wrote into the
// main worktree leaves no residue and never gets a silent success.
//
// The main worktree is recorded as a manifest of its tracked and untracked
// (non-ignored) files. Files the index and HEAD can reproduce are restored
// from HEAD; untracked files and files that were already modified or staged
// at Fork are backed up in memory, because HEAD cannot reproduce them.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// workFile is one main-worktree file in a snapshot.
type workFile struct {
	mode   fs.FileMode
	hash   string // content hash, or link target hash for a symlink
	link   string // symlink target ("" for a regular file)
	absent bool   // tracked but deleted at snapshot time
	data   []byte // backup when HEAD cannot reproduce the content
}

// hookFile is one file under .git/hooks.
type hookFile struct {
	data []byte
	mode fs.FileMode
}

// worldSnapshot is the repository state before one Fork.
type worldSnapshot struct {
	refs   map[string]string // non-world refs: refname -> oid
	head   []byte            // .git/HEAD
	config []byte            // .git/config (nil: absent)
	index  []byte            // .git/index (nil: absent)
	hooks  map[string]hookFile
	files  map[string]workFile // main worktree, slash paths relative to the top
}

// captureSnapshot records the shared repository state before a worktree is
// added. It only reads: it must not disturb the filesystem state fsState
// compares.
func (w *GitWorld) captureSnapshot(ctx context.Context) (*worldSnapshot, error) {
	env := w.backendEnv()
	sn := &worldSnapshot{
		refs:  map[string]string{},
		hooks: map[string]hookFile{},
		files: map[string]workFile{},
	}

	out, _, err := w.s.git(ctx, env, w.s.repo,
		"for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return nil, fmt.Errorf("world: snapshot refs: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || strings.HasPrefix(f[0], "refs/heads/court/") {
			continue
		}
		sn.refs[f[0]] = f[1]
	}
	if sn.head, err = os.ReadFile(filepath.Join(w.s.gitDir, "HEAD")); err != nil {
		return nil, fmt.Errorf("world: snapshot HEAD: %v", err)
	}
	if sn.config, err = readOptional(filepath.Join(w.s.gitDir, "config")); err != nil {
		return nil, fmt.Errorf("world: snapshot config: %v", err)
	}
	if sn.index, err = readOptional(filepath.Join(w.s.gitDir, "index")); err != nil {
		return nil, fmt.Errorf("world: snapshot index: %v", err)
	}
	if sn.hooks, err = readTree(filepath.Join(w.s.gitDir, "hooks")); err != nil {
		return nil, fmt.Errorf("world: snapshot hooks: %v", err)
	}
	if sn.files, err = w.captureMainWorktree(ctx); err != nil {
		return nil, err
	}
	return sn, nil
}

func readOptional(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return raw, err
}

// readTree reads every regular file under dir as path -> content+mode.
func readTree(dir string) (map[string]hookFile, error) {
	files := map[string]hookFile{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		files[filepath.ToSlash(rel)] = hookFile{data: raw, mode: info.Mode()}
		return nil
	})
	return files, err
}

// worktreePaths lists the main worktree's tracked and untracked (non-ignored)
// files, in the -z form so unusual names survive.
func (s *store) worktreePaths(ctx context.Context, env []string) ([]string, error) {
	out, _, err := s.git(ctx, env, s.repo,
		"ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	raw := strings.Split(out, "\x00")
	paths := make([]string, 0, len(raw))
	dd, _ := filepath.Rel(s.repo, s.dataDir)
	dd = filepath.ToSlash(dd)
	for _, p := range raw {
		if p == "" {
			continue
		}
		p = filepath.ToSlash(p)
		if dd != "" && dd != "." && !strings.HasPrefix(dd, "../") &&
			(p == dd || strings.HasPrefix(p, dd+"/")) {
			continue // the data directory can live inside the repository
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// captureMainWorktree records the main worktree files and backs up the
// contents HEAD cannot reproduce.
func (w *GitWorld) captureMainWorktree(ctx context.Context) (map[string]workFile, error) {
	env := w.backendEnv()
	paths, err := w.s.worktreePaths(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("world: snapshot worktree: %v", err)
	}
	// Dirty (staged or unstaged) tracked paths: their content is not in
	// HEAD. diff-index reads the index without refreshing it.
	dirty := map[string]bool{}
	out, _, err := w.s.git(ctx, env, w.s.repo,
		"diff-index", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return nil, fmt.Errorf("world: snapshot diff-index: %v", err)
	}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			dirty[filepath.ToSlash(p)] = true
		}
	}
	// Untracked paths are not in HEAD either.
	out, _, err = w.s.git(ctx, env, w.s.repo,
		"ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("world: snapshot untracked: %v", err)
	}
	untracked := map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			untracked[filepath.ToSlash(p)] = true
		}
	}

	files := make(map[string]workFile, len(paths))
	for _, rel := range paths {
		p := filepath.Join(w.s.repo, filepath.FromSlash(rel))
		info, err := os.Lstat(p)
		if err != nil {
			if os.IsNotExist(err) {
				files[rel] = workFile{absent: true}
				continue
			}
			return nil, fmt.Errorf("world: snapshot %s: %v", rel, err)
		}
		wf := workFile{mode: info.Mode()}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return nil, fmt.Errorf("world: snapshot link %s: %v", rel, err)
			}
			wf.link = target
			wf.hash = hashString(target)
			files[rel] = wf
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("world: snapshot %s: %v", rel, err)
		}
		wf.hash = hashBytes(raw)
		if dirty[rel] || untracked[rel] {
			wf.data = raw
		}
		files[rel] = wf
	}
	return files, nil
}

// restoreSnapshot puts the shared state back. It rewrites only what
// differs, so the filesystem listing of a clean discard is unchanged.
func (w *GitWorld) restoreSnapshot(ctx context.Context, sn *worldSnapshot) error {
	env := w.backendEnv()
	if err := w.restoreHooks(sn); err != nil {
		return err
	}
	if err := w.restoreFile(filepath.Join(w.s.gitDir, "config"), sn.config); err != nil {
		return fmt.Errorf("world: discard %s: config: %v", w.id, err)
	}
	if err := w.restoreFile(filepath.Join(w.s.gitDir, "index"), sn.index); err != nil {
		return fmt.Errorf("world: discard %s: index: %v", w.id, err)
	}
	if err := w.restoreRefs(ctx, env, sn); err != nil {
		return err
	}
	if err := w.restoreFile(filepath.Join(w.s.gitDir, "HEAD"), sn.head); err != nil {
		return fmt.Errorf("world: discard %s: HEAD: %v", w.id, err)
	}
	if err := w.restoreMainWorktree(ctx, env, sn); err != nil {
		return err
	}
	return nil
}

func (w *GitWorld) restoreFile(path string, want []byte) error {
	cur, err := readOptional(path)
	if err != nil {
		return err
	}
	if want == nil {
		if cur == nil {
			return nil
		}
		return os.Remove(path)
	}
	if cur != nil && string(cur) == string(want) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, want, 0o600)
}

func (w *GitWorld) restoreHooks(sn *worldSnapshot) error {
	dir := filepath.Join(w.s.gitDir, "hooks")
	cur, err := readTree(dir)
	if err != nil {
		return fmt.Errorf("world: discard %s: hooks: %v", w.id, err)
	}
	for rel := range cur {
		if _, ok := sn.hooks[rel]; !ok {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				return fmt.Errorf("world: discard %s: hooks: %v", w.id, err)
			}
		}
	}
	for rel, want := range sn.hooks {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		got, err := os.ReadFile(p)
		if err == nil && string(got) == string(want.data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("world: discard %s: hooks: %v", w.id, err)
		}
		if err := os.WriteFile(p, want.data, want.mode.Perm()); err != nil {
			return fmt.Errorf("world: discard %s: hooks: %v", w.id, err)
		}
		_ = os.Chmod(p, want.mode.Perm())
	}
	return nil
}

// restoreRefs removes refs the repository did not have before Fork and
// restores the ones it had. World branches (refs/heads/court/...) are not
// part of the snapshot: each Discard deletes its own branch, and branches
// held by live sibling worktrees are left alone.
func (w *GitWorld) restoreRefs(ctx context.Context, env []string, sn *worldSnapshot) error {
	protected := map[string]bool{}
	out, _, err := w.s.git(ctx, env, w.s.repo, "worktree", "list", "--porcelain")
	if err != nil {
		return fmt.Errorf("world: discard %s: worktrees: %v", w.id, err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "branch ") {
			protected[strings.TrimPrefix(line, "branch ")] = true
		}
	}
	out, _, err = w.s.git(ctx, env, w.s.repo,
		"for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return fmt.Errorf("world: discard %s: refs: %v", w.id, err)
	}
	current := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 {
			current[f[0]] = f[1]
		}
	}
	for ref := range current {
		if _, ok := sn.refs[ref]; ok || protected[ref] {
			continue
		}
		if _, _, err := w.s.git(ctx, env, w.s.repo, "update-ref", "-d", ref); err != nil {
			return fmt.Errorf("world: discard %s: remove ref %s: %v", w.id, ref, err)
		}
	}
	for ref, oid := range sn.refs {
		if current[ref] == oid {
			continue
		}
		if _, _, err := w.s.git(ctx, env, w.s.repo, "update-ref", ref, oid); err != nil {
			return fmt.Errorf("world: discard %s: restore ref %s: %v", w.id, ref, err)
		}
	}
	return nil
}

// restoreMainWorktree removes files a world added to the main worktree and
// puts back the ones it changed or deleted.
func (w *GitWorld) restoreMainWorktree(ctx context.Context, env []string, sn *worldSnapshot) error {
	paths, err := w.s.worktreePaths(ctx, env)
	if err != nil {
		return fmt.Errorf("world: discard %s: worktree list: %v", w.id, err)
	}
	present := map[string]bool{}
	for _, rel := range paths {
		present[rel] = true
		if _, ok := sn.files[rel]; !ok {
			if err := os.RemoveAll(filepath.Join(w.s.repo, filepath.FromSlash(rel))); err != nil {
				return fmt.Errorf("world: discard %s: remove %s: %v", w.id, rel, err)
			}
		}
	}
	for rel, want := range sn.files {
		p := filepath.Join(w.s.repo, filepath.FromSlash(rel))
		if want.absent {
			if err := os.RemoveAll(p); err != nil {
				return fmt.Errorf("world: discard %s: restore absence of %s: %v", w.id, rel, err)
			}
			continue
		}
		if present[rel] {
			info, err := os.Lstat(p)
			if err == nil && wfMatches(p, info, want) {
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("world: discard %s: restore %s: %v", w.id, rel, err)
		}
		if want.link != "" {
			_ = os.RemoveAll(p)
			if err := os.Symlink(want.link, p); err != nil {
				return fmt.Errorf("world: discard %s: restore link %s: %v", w.id, rel, err)
			}
			continue
		}
		data := want.data
		if data == nil {
			out, _, err := w.s.git(ctx, env, w.s.repo, "cat-file", "blob", "HEAD:"+rel)
			if err != nil {
				return fmt.Errorf("world: discard %s: restore %s from HEAD: %v", w.id, rel, err)
			}
			data = []byte(out)
		}
		_ = os.RemoveAll(p)
		if err := os.WriteFile(p, data, want.mode.Perm()); err != nil {
			return fmt.Errorf("world: discard %s: restore %s: %v", w.id, rel, err)
		}
		_ = os.Chmod(p, want.mode.Perm())
	}
	return nil
}

func wfMatches(p string, info fs.FileInfo, want workFile) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(p)
		return want.link != "" && err == nil && target == want.link
	}
	if want.link != "" {
		return false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return hashBytes(raw) == want.hash
}

func hashBytes(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func hashString(s string) string { return hashBytes([]byte(s)) }

// stateLines is the part of the H3 digest added for the shared state a
// world can change (red-team B3-04/05/06): the repository config, every
// file under .git/hooks, and every tracked or untracked file in the main
// worktree. Refs and reachable objects are already hashed by Digest.
func (w *GitWorld) stateLines(ctx context.Context, env []string) ([]string, error) {
	var lines []string
	cfg, err := readOptional(filepath.Join(w.s.gitDir, "config"))
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		lines = append(lines, "C absent")
	} else {
		lines = append(lines, "C "+hashBytes(cfg))
	}
	hooks, err := readTree(filepath.Join(w.s.gitDir, "hooks"))
	if err != nil {
		return nil, err
	}
	var hnames []string
	for rel := range hooks {
		hnames = append(hnames, rel)
	}
	sort.Strings(hnames)
	for _, rel := range hnames {
		lines = append(lines, fmt.Sprintf("H %s %o %s", rel,
			hooks[rel].mode.Perm(), hashBytes(hooks[rel].data)))
	}
	paths, err := w.s.worktreePaths(ctx, env)
	if err != nil {
		return nil, err
	}
	for _, rel := range paths {
		p := filepath.Join(w.s.repo, filepath.FromSlash(rel))
		info, err := os.Lstat(p)
		if err != nil {
			if os.IsNotExist(err) {
				lines = append(lines, "M "+rel+" absent")
				continue
			}
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf("M %s link %s", rel, hashString(target)))
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf("M %s %o %s", rel,
			info.Mode().Perm(), hashBytes(raw)))
	}
	return lines, nil
}
