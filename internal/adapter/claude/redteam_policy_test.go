package claude

// Permanent red-team tests for B3-08 (finding 08): the data-directory
// deny rule must compare resolved file identity, not a string prefix.
// They run under C1.7.2's `-run '^TestOutwardDeny_'` pass.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func rtPolicyLayout(t *testing.T) (datadir, world string) {
	t.Helper()
	datadir = filepath.Join(t.TempDir(), "court-data")
	world = filepath.Join(datadir, "worlds", "w1")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	return datadir, world
}

func rtPolicyCanary(t *testing.T, datadir string) string {
	t.Helper()
	canary := filepath.Join(datadir, "canary.txt")
	if err := os.WriteFile(canary, []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return canary
}

// rtPolicyDenied asserts the decision for path and that the alias really
// writes the canary before the policy is asked.
func rtPolicyDenied(t *testing.T, p Policy, world, alias, canary, content string) {
	t.Helper()
	if alias != canary {
		if err := os.WriteFile(alias, []byte(content), 0o644); err != nil {
			t.Fatalf("OS does not resolve the alias: %v", err)
		}
		got, err := os.ReadFile(canary)
		if err != nil || string(got) != content {
			t.Fatalf("alias does not write the canary: %q %v", got, err)
		}
	}
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": alias}, world); d != "deny" {
		t.Fatalf("data-directory alias not denied: decision=%q reason=%q path=%q",
			d, reason, alias)
	}
}

func TestOutwardDeny_DataDirExactAndWorldExemption(t *testing.T) {
	datadir, world := rtPolicyLayout(t)
	canary := rtPolicyCanary(t, datadir)
	p := Policy{DataDir: datadir}
	if d, _ := p.DecideHook("Write", map[string]any{"file_path": canary}, world); d != "deny" {
		t.Fatalf("exact data-directory path not denied: %q", d)
	}
	inWorld := filepath.Join(world, "ok.txt")
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": inWorld}, world); d != "" {
		t.Fatalf("a path inside the world was denied: %q %q", d, reason)
	}
}

func TestOutwardDeny_DataDirCaseVariant(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive filesystem")
	}
	datadir, world := rtPolicyLayout(t)
	canary := rtPolicyCanary(t, datadir)
	p := Policy{DataDir: datadir}
	rtPolicyDenied(t, p, world, strings.ToUpper(canary), canary, "CANARY-CASE\n")
}

func TestOutwardDeny_DataDirVerbatimPrefix(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("verbatim paths")
	}
	datadir, world := rtPolicyLayout(t)
	canary := rtPolicyCanary(t, datadir)
	p := Policy{DataDir: datadir}
	rtPolicyDenied(t, p, world, `\\?\`+canary, canary, "CANARY-VERBATIM\n")
}

func TestOutwardDeny_DataDirTrailingDot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path normalisation")
	}
	datadir, world := rtPolicyLayout(t)
	canary := rtPolicyCanary(t, datadir)
	p := Policy{DataDir: datadir}
	alias := datadir + "." + string(filepath.Separator) + "canary.txt"
	rtPolicyDenied(t, p, world, alias, canary, "CANARY-DOT\n")
}

func TestOutwardDeny_DataDirJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction")
	}
	datadir, world := rtPolicyLayout(t)
	canary := rtPolicyCanary(t, datadir)
	link := filepath.Join(world, "j")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, datadir).CombinedOutput(); err != nil {
		t.Fatalf("cannot create junction: %v: %s", err, out)
	}
	p := Policy{DataDir: datadir}
	rtPolicyDenied(t, p, world, filepath.Join(link, "canary.txt"), canary, "CANARY-JUNCTION\n")
}

func TestOutwardDeny_DataDirSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("socket-level symlinks need developer mode; the junction case covers Windows")
	}
	datadir, world := rtPolicyLayout(t)
	canary := rtPolicyCanary(t, datadir)
	link := filepath.Join(world, "s")
	if err := os.Symlink(datadir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	p := Policy{DataDir: datadir}
	rtPolicyDenied(t, p, world, filepath.Join(link, "canary.txt"), canary, "CANARY-SYMLINK\n")
}
