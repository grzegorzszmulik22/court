//go:build windows

package claude

// Path identity for the data-directory deny rule on Windows. NTFS is
// case-insensitive, accepts the \?\ verbatim prefix, strips trailing
// dots and spaces from components, expands 8.3 short names, and follows
// junctions and symlinks. A cleaned-string prefix misses all of those, so
// the policy resolves the path through the filesystem instead
// (GetFinalPathNameByHandle) and compares case-insensitively.

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// canonicalPath resolves path to the filesystem's own spelling. For a
// path that does not exist yet, the deepest existing ancestor is resolved
// and the remaining components are appended, which is where a Write will
// land. It returns an error when the file system cannot be asked; the
// caller then falls back to its lexical test.
func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	cur := abs
	var rest []string
	for {
		p, err := windows.UTF16PtrFromString(cur)
		if err != nil {
			return "", err
		}
		h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err == nil {
			resolved, err := finalPath(h)
			windows.CloseHandle(h)
			if err != nil {
				return "", err
			}
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved, nil
		}
		if err != windows.ERROR_FILE_NOT_FOUND && err != windows.ERROR_PATH_NOT_FOUND {
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

// finalPath asks Windows for the normalized DOS path of an open handle.
func finalPath(h windows.Handle) (string, error) {
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", fmt.Errorf("empty final path")
	}
	p := windows.UTF16ToString(buf[:n])
	switch {
	case strings.HasPrefix(p, `\?\UNC\`):
		p = `\` + strings.TrimPrefix(p, `\?\UNC\`)
	case strings.HasPrefix(p, `\?\`):
		p = strings.TrimPrefix(p, `\?\`)
	}
	return filepath.Clean(p), nil
}

// pathsEqualFold compares resolved paths the way NTFS does.
func pathsEqualFold(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// pathHasPrefixFold reports a directory containment test on resolved paths.
func pathHasPrefixFold(cand, dir string) bool {
	c := strings.ToLower(filepath.Clean(cand))
	d := strings.ToLower(filepath.Clean(dir))
	return strings.HasPrefix(c, d+string(filepath.Separator))
}
