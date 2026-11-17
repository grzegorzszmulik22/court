package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// WithinDir reports whether target, resolved the way the filesystem would
// resolve it, stays inside root. A relative target is interpreted under
// root. The check is two-layered (B3-09):
//
//   - lexical: filepath.Clean(target) must stay under root — a `..`
//     that climbs out fails here;
//   - symlink: the longest existing prefix of the target is resolved
//     with filepath.EvalSymlinks and the resolved tail re-attached — a
//     symlink inside root pointing outside, or `..` crossing a symlink,
//     fails here.
//
// When the path stays inside the directory the returned reason is empty;
// otherwise it names the violation for a denial record.
func WithinDir(root, target string) (bool, string) {
	if target == "" {
		return false, "empty path"
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	cleanRoot := filepath.Clean(root)
	clean := filepath.Clean(target)
	if !under(clean, cleanRoot) {
		return false, "path leaves the directory"
	}
	rroot, err := resolvePath(cleanRoot)
	if err != nil {
		return false, "cannot resolve directory: " + err.Error()
	}
	rtarget, err := resolvePath(target)
	if err != nil {
		return false, "cannot resolve path: " + err.Error()
	}
	if !under(filepath.Clean(rtarget), rroot) {
		return false, "path leaves the directory through a symlink"
	}
	return true, ""
}

// under reports whether p is root or below it.
func under(p, root string) bool {
	return p == root ||
		strings.HasPrefix(p, root+string(os.PathSeparator))
}

// resolvePath resolves symlinks along the longest existing prefix of p
// and re-attaches the not-yet-existing tail, so containment holds for
// targets that do not exist yet.
func resolvePath(p string) (string, error) {
	dir := p
	var tail string
	for {
		r, err := filepath.EvalSymlinks(dir)
		if err == nil {
			if tail == "" {
				return r, nil
			}
			return filepath.Join(r, tail), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		if tail == "" {
			tail = filepath.Base(dir)
		} else {
			tail = filepath.Join(filepath.Base(dir), tail)
		}
		dir = parent
	}
}
