package claude

// Red-team reproductions for the PreToolUse data-directory policy (brief B3,
// aim 3). spec/02 §2: "The adapter denies any file-tool call whose path is
// inside the data directory". The implementation compares cleaned path
// strings; on Windows the same file has other spellings, and those are not
// recognised.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func rtBase(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(file))))
	dir := filepath.Join(root, ".rt", "scratch", "policy")
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func rtLayout(t *testing.T) (datadir, world string) {
	t.Helper()
	datadir = filepath.Join(rtBase(t), "court-data")
	world = filepath.Join(datadir, "worlds", "w1")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	return datadir, world
}

// TestRT_PolicyCaseVariantBypass: NTFS is case-insensitive, but `inside`
// compares strings case-sensitively, so an upper-cased path into the data
// directory is not denied and the write lands on the real file.
func TestRT_PolicyCaseVariantBypass(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive filesystem")
	}
	datadir, world := rtLayout(t)
	canary := filepath.Join(datadir, "canary.txt")
	if err := os.WriteFile(canary, []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Policy{DataDir: datadir}
	if d, _ := p.DecideHook("Write", map[string]any{"file_path": canary}, world); d != "deny" {
		t.Fatalf("exact data-directory path not denied: %q", d)
	}
	upper := strings.ToUpper(canary)
	if err := os.WriteFile(upper, []byte("CANARY-OVERWRITE\n"), 0o644); err != nil {
		t.Fatalf("OS does not treat the variant as the same file: %v", err)
	}
	got, _ := os.ReadFile(canary)
	if string(got) != "CANARY-OVERWRITE\n" {
		t.Fatalf("canary content = %q", got)
	}
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": upper}, world); d != "deny" {
		t.Fatalf("case-varied data-directory path not denied: decision=%q reason=%q path=%q",
			d, reason, upper)
	}
}

// TestRT_Policy8Dot3Bypass: the short (8.3) spelling of the same directory
// is not recognised either.
func TestRT_Policy8Dot3Bypass(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("8.3 names")
	}
	datadir, world := rtLayout(t)
	canary := filepath.Join(datadir, "canary.txt")
	if err := os.WriteFile(canary, []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	short := rtShortPath(t, datadir)
	if short == "" || strings.EqualFold(short, datadir) {
		t.Skipf("8.3 short names are disabled on this volume (no short name for %s)", datadir)
	}
	if _, err := os.Stat(short); err != nil {
		t.Skipf("short name %q does not resolve: %v", short, err)
	}
	p := Policy{DataDir: datadir}
	alias := filepath.Join(short, "canary.txt")
	if err := os.WriteFile(alias, []byte("CANARY-8DOT3\n"), 0o644); err != nil {
		t.Fatalf("OS cannot write the alias: %v", err)
	}
	got, _ := os.ReadFile(canary)
	if string(got) != "CANARY-8DOT3\n" {
		t.Fatalf("canary content = %q", got)
	}
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": alias}, world); d != "deny" {
		t.Fatalf("8.3 alias of the data directory not denied: decision=%q reason=%q path=%q",
			d, reason, alias)
	}
}

// TestRT_PolicyJunctionBypass: a directory junction inside the world is an
// allowed path that resolves into the data directory; the file-tool rule
// does not resolve it, so a Write through the junction reaches the ledger.
func TestRT_PolicyJunctionBypass(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction")
	}
	datadir, world := rtLayout(t)
	canary := filepath.Join(datadir, "canary.txt")
	if err := os.WriteFile(canary, []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(world, "j")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, datadir).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, out)
	}
	viaLink := filepath.Join(link, "canary.txt")
	p := Policy{DataDir: datadir}
	if err := os.WriteFile(viaLink, []byte("CANARY-JUNCTION\n"), 0o644); err != nil {
		t.Fatalf("write through junction: %v", err)
	}
	got, _ := os.ReadFile(canary)
	if string(got) != "CANARY-JUNCTION\n" {
		t.Fatalf("canary content = %q", got)
	}
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": viaLink}, world); d != "deny" {
		t.Fatalf("junction path into the data directory not denied: decision=%q reason=%q path=%q",
			d, reason, viaLink)
	}
}

// TestRT_PolicyVerbatimPrefixBypass: the \\?\ verbatim prefix names the same
// file but is not a string prefix of the data directory.
func TestRT_PolicyVerbatimPrefixBypass(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("verbatim paths")
	}
	datadir, world := rtLayout(t)
	canary := filepath.Join(datadir, "canary.txt")
	if err := os.WriteFile(canary, []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verbatim := `\\?\` + canary
	p := Policy{DataDir: datadir}
	if err := os.WriteFile(verbatim, []byte("CANARY-VERBATIM\n"), 0o644); err != nil {
		t.Fatalf("verbatim write: %v", err)
	}
	got, _ := os.ReadFile(canary)
	if string(got) != "CANARY-VERBATIM\n" {
		t.Fatalf("canary content = %q", got)
	}
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": verbatim}, world); d != "deny" {
		t.Fatalf("verbatim data-directory path not denied: decision=%q reason=%q path=%q",
			d, reason, verbatim)
	}
}

func rtShortPath(t *testing.T, p string) string {
	t.Helper()
	script := `Write-Output (New-Object -ComObject Scripting.FileSystemObject).GetFolder('` +
		strings.ReplaceAll(p, "'", "''") + `').ShortPath`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// TestRT_PolicyTrailingDotBypass: Windows strips a trailing dot from a path
// component, so <data-dir>.\canary.txt names the same file while the string
// prefix test sees a different directory.
func TestRT_PolicyTrailingDotBypass(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path normalisation")
	}
	datadir, world := rtLayout(t)
	canary := filepath.Join(datadir, "canary.txt")
	if err := os.WriteFile(canary, []byte("ORIGINAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := datadir + "." + string(filepath.Separator) + "canary.txt"
	if err := os.WriteFile(alias, []byte("CANARY-DOT\n"), 0o644); err != nil {
		t.Fatalf("OS does not treat the trailing-dot alias as the same file: %v", err)
	}
	got, _ := os.ReadFile(canary)
	if string(got) != "CANARY-DOT\n" {
		t.Fatalf("canary content = %q", got)
	}
	p := Policy{DataDir: datadir}
	if d, reason := p.DecideHook("Write", map[string]any{"file_path": alias}, world); d != "deny" {
		t.Fatalf("trailing-dot data-directory alias not denied: decision=%q reason=%q path=%q",
			d, reason, alias)
	}
}
