package scripted_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"court/internal/adapter"
	"court/internal/adapter/claude"
	"court/internal/adapter/scripted"
)

// TestC1_7_1: generated settings and MCP configuration validate against
// the documented shapes, and are written under the data directory, never
// the repository.
func TestC1_7_1(t *testing.T) {
	s := adapter.Session{
		Agent: "a1", Commission: "c1", Token: "tok123",
		HookURL: "http://127.0.0.1:9999/hook/tok123",
		MCPURL:  "http://127.0.0.1:9999/mcp/tok123",
		DataDir: t.TempDir(),
	}
	settings := claude.Settings(s)
	hooks, _ := settings["hooks"].(map[string]any)
	for _, ev := range claude.HookEvents {
		entries, _ := hooks[ev].([]any)
		if len(entries) != 1 {
			t.Fatalf("hooks.%s = %v", ev, entries)
		}
		e := entries[0].(map[string]any)
		if e["matcher"] != "*" {
			t.Fatalf("%s matcher = %v", ev, e["matcher"])
		}
		hs := e["hooks"].([]any)
		h0 := hs[0].(map[string]any)
		if h0["type"] != "http" || h0["url"] != s.HookURL {
			t.Fatalf("%s handler = %v", ev, h0)
		}
	}
	allowed := settings["allowedHttpHookUrls"].([]string)
	if len(allowed) != 1 || !strings.HasPrefix(s.HookURL, allowed[0]) {
		t.Fatalf("allowedHttpHookUrls = %v", allowed)
	}
	mcp := claude.MCPConfig(s)
	court := mcp["mcpServers"].(map[string]any)["court"].(map[string]any)
	if court["type"] != "http" || court["url"] != s.MCPURL {
		t.Fatalf("mcpServers.court = %v", court)
	}
	// Files land inside DataDir/agents/<agent>/ and parse back.
	settingsPath, mcpCfg, err := claude.WriteConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{settingsPath, mcpCfg} {
		if !strings.HasPrefix(p, s.DataDir) {
			t.Fatalf("config written outside data dir: %s", p)
		}
	}
}

// TestC1_7_2: every outward-list entry and every data-directory path is
// denied in the documented response shape; an ordinary edit and `go test`
// abstain.
func TestC1_7_2(t *testing.T) {
	h := newHarness(t)
	c, err := h.AddAgent(&scripted.AgentSpec{ID: "w1", Parent: "human"})
	if err != nil {
		t.Fatal(err)
	}
	dd := h.DataDir
	denied := []struct {
		tool  string
		input map[string]any
	}{
		{"WebFetch", map[string]any{"url": "https://x"}},
		{"WebSearch", map[string]any{"query": "q"}},
		{"mcp__other__x", map[string]any{}},
		{"Bash", map[string]any{"command": "curl https://e"}},
		{"Bash", map[string]any{"command": "wget https://e"}},
		{"Bash", map[string]any{"command": "ssh h"}},
		{"Bash", map[string]any{"command": "scp a b:"}},
		{"Bash", map[string]any{"command": "git push"}},
		{"Bash", map[string]any{"command": "npm publish"}},
		{"Bash", map[string]any{"command": "aws s3 ls"}},
		{"Bash", map[string]any{"command": "echo ok; curl https://e"}},
		{"Bash", map[string]any{"command": "cat " + dd + "/ledger.db"}},
		{"Read", map[string]any{"file_path": dd + "/ledger.db"}},
		{"Edit", map[string]any{"file_path": c.Worktree + "/../ledger.db", "old_string": "a", "new_string": "b"}},
	}
	for _, tc := range denied {
		_, wasDenied, err := c.PreToolUse(tc.tool, tc.input, "", "")
		if err != nil {
			t.Fatalf("%s %v: %v", tc.tool, tc.input, err)
		}
		if !wasDenied {
			t.Fatalf("%s %v was not denied", tc.tool, tc.input)
		}
	}
	allowed := []struct {
		tool  string
		input map[string]any
	}{
		{"Edit", map[string]any{"file_path": c.Worktree + "/main.go", "old_string": "a", "new_string": "b"}},
		{"Bash", map[string]any{"command": "go test ./..."}},
	}
	for _, tc := range allowed {
		_, wasDenied, err := c.PreToolUse(tc.tool, tc.input, "", "")
		if err != nil {
			t.Fatalf("%s %v: %v", tc.tool, tc.input, err)
		}
		if wasDenied {
			t.Fatalf("%s %v was denied", tc.tool, tc.input)
		}
	}
	// Denials are recorded as tool.event with a reason.
	for _, e := range toolEvents(t, h) {
		if strOf(bodyField(t, e, "outcome")) == "denied" {
			if strOf(bodyField(t, e, "reason")) == "" {
				t.Fatalf("denied event without reason: %v", e.Body)
			}
		}
	}
}

// TestC1_7_3: the adapter starts a stand-in executable with the world as
// working directory and the documented flags, asserted from the double's
// own log.
func TestC1_7_3(t *testing.T) {
	dir := t.TempDir()
	world := t.TempDir()
	logPath := filepath.Join(dir, "double.log")
	// The double records its cwd and argv, prints a stream-json line and
	// drains stdin until EOF so Send and Stop are exercised. It is a Go
	// program built here, so the test runs on every OS Court supports
	// (a shell script cannot be started directly on Windows).
	double := buildDouble(t, dir, logPath)
	a := &claude.Adapter{Bin: double}
	h, err := a.Start(stubWorld{world}, "the commission prompt",
		adapter.Session{Agent: "a1", Token: "tok9", DataDir: dir,
			HookURL: "http://127.0.0.1:1/hook/tok9",
			MCPURL:  "http://127.0.0.1:1/mcp/tok9"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Send(h, "follow up"); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(h); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	if !strings.Contains(log, "cwd="+world) {
		t.Fatalf("double did not run in the world:\n%s", log)
	}
	for _, flag := range []string{
		"-p the commission prompt",
		"--output-format stream-json", "--verbose",
		"--input-format stream-json", "--session-id tok9",
		"--settings ", "--mcp-config ", "--strict-mcp-config",
		"--permission-mode default",
	} {
		if !strings.Contains(log, flag) {
			t.Fatalf("double args missing %q:\n%s", flag, log)
		}
	}
	// The generated files exist under the data dir, and no settings or
	// hook configuration of any name leaked into the world.
	if _, err := os.Stat(filepath.Join(dir, "agents", "a1", "settings.json")); err != nil {
		t.Fatal("settings.json not generated under data dir")
	}
	err = filepath.Walk(world, func(p string, _ os.FileInfo, _ error) error {
		base := filepath.Base(p)
		if base == "settings.json" || base == ".claude" || base == "mcp.json" {
			t.Fatalf("configuration leaked into the world: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type stubWorld struct{ path string }

func (s stubWorld) Path() string { return s.path }

// buildDouble compiles the stand-in claude executable into dir. It appends
// "cwd=<dir>" and "args=<argv joined by spaces>" lines to logPath.
func buildDouble(t *testing.T, dir, logPath string) string {
	t.Helper()
	src := filepath.Join(dir, "double")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	prog := `package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	f, err := os.OpenFile(` + strconv.Quote(logPath) + `, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(2)
	}
	wd, _ := os.Getwd()
	fmt.Fprintf(f, "cwd=%s\nargs=%s\n", wd, strings.Join(os.Args[1:], " "))
	if err := f.Close(); err != nil {
		os.Exit(2)
	}
	fmt.Println(` + "`" + `{"type":"system","subtype":"init"}` + "`" + `)
	io.Copy(io.Discard, os.Stdin)
}
`
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module double\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "claude-double")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
	cmd := exec.Command(gobin, "build", "-o", bin, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build claude double: %v\n%s", err, out)
	}
	return bin
}
