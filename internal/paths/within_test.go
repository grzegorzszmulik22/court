package paths

// B3-09: confinement is by whole path component after normalisation, so a
// sibling that merely shares a string prefix (<root>-x, <root>2) is
// outside, and on a case-insensitive filesystem a case variant of the
// root is inside while a case variant of a sibling stays outside.

import (
	"os"
	"path/filepath"
	"testing"
)

// A component-boundary check is what separates confinement from a raw
// string-prefix test: each of these must answer correctly, or a
// strings.HasPrefix(p, root) sabotage passes.
func TestC1_5_7_WithinDirComponents(t *testing.T) {
	base := t.TempDir()
	world := filepath.Join(base, "world")
	for _, dir := range []string{world,
		filepath.Join(base, "world-x"),
		filepath.Join(base, "world2")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"root itself", world, true},
		{"child", filepath.Join(world, "file"), true},
		{"grandchild", filepath.Join(world, "sub", "file"), true},
		{"climb back in", filepath.Join(world, "..", "world", "file"), true},
		{"sibling -x", filepath.Join(base, "world-x", "file"), false},
		{"sibling 2", filepath.Join(base, "world2", "file"), false},
		{"sibling .bak", filepath.Join(base, "world.bak", "file"), false},
		{"../world-x", filepath.Join(world, "..", "world-x", "f"), false},
		{"../world2", filepath.Join(world, "..", "world2", "f"), false},
		{"climb out", filepath.Join(world, "..", "..", "f"), false},
		{"absolute outside", filepath.Join(base, "outside", "f"), false},
		{"relative child", "file", true},
		{"relative ../", filepath.Join("..", "world-x", "f"), false},
	}
	for _, c := range cases {
		got, reason := withinDir(world, c.target, false)
		if got != c.want {
			t.Errorf("%s: withinDir(%q)=%v (%s), want %v",
				c.name, c.target, got, reason, c.want)
		}
	}
}

// The under() primitive itself: the boundary must be a whole component,
// not any shared prefix. This is the exact property a string-prefix
// sabotage breaks.
func TestC1_5_7_UnderComponentBoundary(t *testing.T) {
	root := string(filepath.Separator) + filepath.Join("base", "world")
	cases := []struct {
		p    string
		want bool
	}{
		{root, true},
		{filepath.Join(root, "f"), true},
		{filepath.Join(root, "a", "b"), true},
		{root + "-x", false},
		{filepath.Join(root+"-x", "f"), false},
		{root + "2", false},
		{filepath.Join(root+"2", "f"), false},
		{root + ".bak", false},
		{filepath.Join(root+"tail", "f"), false},
		{string(filepath.Separator) + filepath.Join("base", "worl"), false},
		{string(filepath.Separator) + filepath.Join("base", "worl", "d"), false},
	}
	for _, c := range cases {
		if got := under(c.p, root, false); got != c.want {
			t.Errorf("under(%q, %q)=%v, want %v", c.p, root, got, c.want)
		}
	}
}

// Windows semantics: names compare case-insensitively per component, so a
// case variant of the root is inside and a case variant of a prefix
// sibling stays outside.
func TestC1_5_7_WithinDirFold(t *testing.T) {
	base := t.TempDir()
	world := filepath.Join(base, "world")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "world-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	upper := filepath.Join(base, "WORLD")
	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"case variant of root", filepath.Join(upper, "file"), true},
		{"root under fold", upper, true},
		{"mixed case child", filepath.Join(upper, "Sub", "F.go"), true},
		{"fold sibling -x", filepath.Join(base, "WORLD-X", "f"), false},
		{"fold sibling 2", filepath.Join(base, "WORLD2", "f"), false},
		{"fold outside", filepath.Join(base, "OTHER", "f"), false},
	}
	for _, c := range cases {
		got, reason := withinDir(world, c.target, true)
		if got != c.want {
			t.Errorf("%s: withinDir(%q, fold)=%v (%s), want %v",
				c.name, c.target, got, reason, c.want)
		}
	}
	// And the boundary table under fold.
	root := upper
	for _, c := range []struct {
		p    string
		want bool
	}{
		{root, true},
		{filepath.Join(root, "f"), true},
		{filepath.Join(root+"-x", "f"), false},
		{filepath.Join(root+"2", "f"), false},
		{filepath.Join(base, "world-x", "f"), false},
	} {
		if got := under(c.p, world, true); got != c.want {
			t.Errorf("under(%q, %q, fold)=%v, want %v", c.p, world, got, c.want)
		}
	}
}

// A symlink inside the root that resolves outside, and a `..` that
// crosses one, are outside even though the lexical path is inside.
func TestC1_5_7_WithinDirSymlinks(t *testing.T) {
	base := t.TempDir()
	world := filepath.Join(base, "world")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{world, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(world, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		filepath.Join(world, "link", "f"),
		filepath.Join(world, "link", "..", "..", "outside", "f"),
	} {
		if ok, reason := withinDir(world, target, false); ok {
			t.Errorf("%q accepted (%s)", target, reason)
		}
	}
	if ok, reason := withinDir(world, filepath.Join(world, "sub", "f"), false); !ok {
		t.Errorf("inside path denied: %s", reason)
	}
}
