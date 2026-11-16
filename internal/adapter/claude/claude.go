// Package claude is the live adapter of spec/02 §4.1: it generates the
// session's settings and MCP configuration under the Court data
// directory, starts Claude Code with the documented flags and the world
// as its working directory, sends follow-up input over the stream-json
// stdin protocol, and stops the process. Tool events never travel
// through this package — they reach the daemon's hook endpoint directly.
package claude

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"court/internal/adapter"
)


// Adapter starts Claude Code sessions. Bin defaults to "claude" and
// exists so hermetic checks can point the adapter at a stand-in
// executable (C1.7.3); it is never the vendor's SDK.
type Adapter struct {
	Bin string
}

// Settings returns the generated settings object of the documented
// shape: http hooks for every consumed event posting to the session's
// hook URL, plus the allowedHttpHookUrls entry they need.
func Settings(s adapter.Session) map[string]any {
	hooks := map[string]any{}
	for _, ev := range HookEvents {
		hooks[ev] = []any{map[string]any{
			"matcher": "*",
			"hooks": []any{map[string]any{
				"type": "http",
				"url":  s.HookURL,
			}},
		}}
	}
	u, _ := url.Parse(s.HookURL)
	return map[string]any{
		"hooks":               hooks,
		"allowedHttpHookUrls": []string{u.Scheme + "://" + u.Host + "/hook/"},
	}
}

// MCPConfig returns the generated MCP configuration object: Court's own
// endpoint and nothing else (strict mode keeps every other server out).
func MCPConfig(s adapter.Session) map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			"court": map[string]any{"type": "http", "url": s.MCPURL},
		},
	}
}

// sessionDir is where the generated files live: the data directory,
// never the repository (sabotage: settings-written-into-repo).
func sessionDir(s adapter.Session) string {
	return filepath.Join(s.DataDir, "agents", s.Agent)
}

// WriteConfig renders settings.json and mcp.json into the session dir
// and returns their paths.
func WriteConfig(s adapter.Session) (settings, mcpConfig string, err error) {
	dir := sessionDir(s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	settings = filepath.Join(dir, "settings.json")
	mcpConfig = filepath.Join(dir, "mcp.json")
	if err := writeJSON(settings, Settings(s)); err != nil {
		return "", "", err
	}
	if err := writeJSON(mcpConfig, MCPConfig(s)); err != nil {
		return "", "", err
	}
	return settings, mcpConfig, nil
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// proc is the running Claude Code process behind a Handle.
type proc struct {
	cmd   *exec.Cmd
	stdin *os.File
	done  atomic.Bool
}

func (p *proc) Done() bool { return p.done.Load() }

// Args is the documented flag list of spec/02 §4.1 (the tool has no
// working-directory flag; the world is the process directory).
func Args(sessionID, settings, mcpConfig, prompt string) []string {
	return []string{
		"-p", prompt,
		"--output-format", "stream-json", "--verbose",
		"--input-format", "stream-json",
		"--session-id", sessionID,
		"--settings", settings,
		"--mcp-config", mcpConfig, "--strict-mcp-config",
		"--permission-mode", "default",
	}
}

// Start writes the session config and starts the process in the world.
func (a *Adapter) Start(w adapter.World, prompt string, s adapter.Session) (adapter.Handle, error) {
	settings, mcpConfig, err := WriteConfig(s)
	if err != nil {
		return nil, err
	}
	bin := a.Bin
	if bin == "" {
		bin = "claude"
	}
	cmd := exec.Command(bin, Args(s.Token, settings, mcpConfig, prompt)...)
	cmd.Dir = w.Path()
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude start: %w", err)
	}
	p := &proc{cmd: cmd}
	if f, ok := stdin.(*os.File); ok {
		p.stdin = f
	}
	go func() { cmd.Wait(); p.done.Store(true) }()
	return p, nil
}

// Send writes one SDKUserMessage line to the session's stdin.
func (a *Adapter) Send(h adapter.Handle, message string) error {
	p := h.(*proc)
	if p.stdin == nil {
		return fmt.Errorf("session has no stdin")
	}
	line, err := json.Marshal(map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": message},
	})
	if err != nil {
		return err
	}
	_, err = p.stdin.Write(append(line, '\n'))
	return err
}

// Stop closes stdin, gives the session a moment to exit cleanly, then
// kills it.
func (a *Adapter) Stop(h adapter.Handle) error {
	p := h.(*proc)
	if p.stdin != nil {
		p.stdin.Close()
	}
	for i := 0; i < 100 && !p.Done(); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if !p.Done() {
		p.cmd.Process.Kill()
	}
	return nil
}
