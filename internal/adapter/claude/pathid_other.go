//go:build !windows

package claude

// Path identity for the data-directory deny rule on case-sensitive
// filesystems: resolve every symlink in the deepest existing ancestor so
// a link inside the world that points into the data directory is seen as
// the data directory itself.

import (
	"os"
	"path/filepath"
)

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cur := abs
	var rest []string
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(abs), nil
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}

func pathsEqualFold(a, b string) bool { return a == b }

func pathHasPrefixFold(cand, dir string) bool {
	return len(cand) > len(dir) && cand[:len(dir)] == dir &&
		cand[len(dir)] == filepath.Separator
}
