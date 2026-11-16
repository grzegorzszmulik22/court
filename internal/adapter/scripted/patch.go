package scripted

import (
	"fmt"
	"strconv"
	"strings"
)

// applyPatch applies one unified diff to content. It handles standard
// `@@ -a[,b] +c[,d] @@` hunks with context, add and remove lines, and the
// new-file form (`--- /dev/null`). This is deliberately small: scripts
// generate their patches from before/after text, so context is always
// exact at the stated line numbers.
func applyPatch(before []byte, patch string) ([]byte, error) {
	hadTrailingNL := len(before) == 0 || before[len(before)-1] == '\n'
	src := splitLines(string(before))
	var out []string
	srcIdx := 0 // 0-based line index into src
	lines := strings.Split(patch, "\n")
	i := 0
	// Skip file headers (---, +++, diff, index).
	for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
		i++
	}
	if i >= len(lines) {
		return nil, fmt.Errorf("patch has no hunks")
	}
	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "@@ ") {
			i++
			continue
		}
		oldStart, _, err := parseHunk(lines[i])
		if err != nil {
			return nil, err
		}
		i++
		// Copy unchanged lines up to the hunk start (oldStart is 1-based;
		// 0 means a pure insertion at the top).
		target := oldStart - 1
		if target < 0 {
			target = 0
		}
		for srcIdx < target && srcIdx < len(src) {
			out = append(out, src[srcIdx])
			srcIdx++
		}
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
			l := lines[i]
			if l == "" && i+1 < len(lines) &&
				(strings.HasPrefix(lines[i+1], "@@ ") || strings.HasPrefix(lines[i+1], "diff ")) {
				i++
				continue
			}
			if l == `\ No newline at end of file` {
				i++
				continue
			}
			switch {
			case strings.HasPrefix(l, " "):
				if srcIdx >= len(src) || src[srcIdx] != l[1:] {
					return nil, fmt.Errorf("context mismatch at line %d: want %q got %q",
						srcIdx+1, l[1:], at(src, srcIdx))
				}
				out = append(out, l[1:])
				srcIdx++
			case strings.HasPrefix(l, "-"):
				if srcIdx >= len(src) || src[srcIdx] != l[1:] {
					return nil, fmt.Errorf("remove mismatch at line %d: want %q got %q",
						srcIdx+1, l[1:], at(src, srcIdx))
				}
				srcIdx++
			case strings.HasPrefix(l, "+"):
				out = append(out, l[1:])
			case l == "":
				// A bare empty line inside a hunk is a context line "".
				if srcIdx < len(src) && src[srcIdx] == "" {
					out = append(out, "")
					srcIdx++
				}
			default:
				return nil, fmt.Errorf("bad patch line %q", l)
			}
			i++
		}
	}
	for srcIdx < len(src) {
		out = append(out, src[srcIdx])
		srcIdx++
	}
	res := strings.Join(out, "\n")
	if hadTrailingNL && res != "" {
		res += "\n"
	}
	return []byte(res), nil
}

func at(src []string, i int) string {
	if i < len(src) {
		return src[i]
	}
	return "<eof>"
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// parseHunk reads "@@ -a[,b] +c[,d] @@" and returns the old start and count.
func parseHunk(h string) (start, count int, err error) {
	end := strings.Index(h[3:], "@@")
	if end < 0 {
		return 0, 0, fmt.Errorf("bad hunk header %q", h)
	}
	fields := strings.Fields(h[3 : 3+end])
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			a, _, ok := strings.Cut(f[1:], ",")
			if start, err = strconv.Atoi(a); err != nil {
				return 0, 0, fmt.Errorf("bad hunk header %q", h)
			}
			if !ok {
				count = 1
				continue
			}
			_, c, _ := strings.Cut(f[1:], ",")
			if count, err = strconv.Atoi(c); err != nil {
				return 0, 0, fmt.Errorf("bad hunk header %q", h)
			}
		}
	}
	return start, count, nil
}
