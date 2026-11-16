package scripted

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"court/internal/adapter/claude"
	"court/internal/tools"
)

// Client is one scripted agent session: it posts hook payloads in the
// documented Claude Code shape and calls the daemon's MCP endpoint. One
// Client is one session token; subagent posts go through the same client
// with agent_id set, like a real session's subagents share its token.
type Client struct {
	// HookURL is the session's /hook/<token> endpoint.
	HookURL string
	// MCPURL is the session's /mcp/<token> endpoint.
	MCPURL string
	// Worktree is the agent's working directory (the world).
	Worktree string
	// SessionID is the tool-side session id placed on every payload.
	SessionID string
	// Agent is the agent id (for logging and default agent_type).
	Agent string
	// GitEnv is the world's per-world git configuration (world.GitEnv()).
	GitEnv []string
	// Transcript is the fabricated transcript path placed on payloads.
	Transcript string
	// HTTP is the transport (nil uses a default client with a timeout).
	HTTP *http.Client
	// Retries is how many times a failed hook post is resent before the
	// step fails; a resent post must never record twice (C1.8.3).
	Retries int
	// Rnd picks the occasional duplicate delivery (nil = never).
	Rnd *rand.Rand
	// Log receives one line per step (nil discards).
	Log io.Writer
	// ProtocolVersion is the MCP protocol revision the client initializes
	// with ("" = the pinned revision).
	ProtocolVersion string

	seq     atomic.Int64
	mcpID   atomic.Int64
	mcpInit atomic.Bool
}

// httpClient returns the HTTP transport with a per-request timeout so a
// wedged endpoint fails a step instead of hanging it.
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) logf(format string, a ...any) {
	if c.Log != nil {
		fmt.Fprintf(c.Log, "scripted %s: "+format+"\n", append([]any{c.Agent}, a...)...)
	}
}

// --------------------------------------------------------------------------
// Hook posting
// --------------------------------------------------------------------------

// postHook delivers one payload to the hook endpoint, retrying transport
// failures (a daemon restart mid-run) up to Retries times. A resent post
// is byte-identical, so the daemon's idempotent append records it once.
func (c *Client) postHook(payload map[string]any) (*http.Response, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	tries := 1 + c.Retries
	var lastErr error
	var lastResp *http.Response
	for i := 0; i < tries; i++ {
		// A scripted agent occasionally duplicates a delivery: hook posts
		// arrive at least once, and the ledger must record them once.
		dups := 1
		if c.Rnd != nil && c.Rnd.Intn(20) == 0 {
			dups = 2
		}
		for j := 0; j < dups; j++ {
			req, err := http.NewRequest("POST", c.HookURL, bytes.NewReader(raw))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := c.httpClient().Do(req)
			if err != nil {
				lastErr = err
				continue
			}
			lastResp = resp
			if resp.StatusCode >= 500 {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				lastErr = fmt.Errorf("hook post: %s", resp.Status)
				continue
			}
			lastErr = nil
		}
		if lastErr == nil {
			return lastResp, nil
		}
		select {
		case <-time.After(time.Duration(50*(i+1)) * time.Millisecond):
		}
	}
	return nil, lastErr
}

// PayloadFor builds a documented-shape payload for a tool call in this
// session (used by checks that post hook bodies directly).
func (c *Client) PayloadFor(event, toolUseID, toolName string,
	input map[string]any, agentID, agentType string) map[string]any {
	return c.payload(event, claude.PayloadParams{
		ToolName: toolName, ToolInput: input, ToolUseID: toolUseID,
		AgentID: agentID, AgentType: agentType,
		ToolResponse: map[string]any{"scripted": true},
	})
}

// payload assembles a documented-shape hook payload around this session.
func (c *Client) payload(event string, p claude.PayloadParams) map[string]any {
	p.SessionID = c.SessionID
	p.TranscriptPath = c.Transcript
	if p.TranscriptPath == "" {
		p.TranscriptPath = filepath.Join(c.Worktree, ".transcript.jsonl")
	}
	p.Cwd = c.Worktree
	p.ScratchpadDir = filepath.Join(c.Worktree, ".scratchpad")
	if p.PermissionMode == "" {
		p.PermissionMode = "default"
	}
	if p.EffortLevel == "" {
		p.EffortLevel = "medium"
	}
	return claude.Payload(event, p)
}

// PreToolUse posts the Pre hook and reports whether the daemon denied it
// (the documented deny shape is recognized; anything else abstains).
func (c *Client) PreToolUse(toolName string, input map[string]any,
	agentID, agentType string) (toolUseID string, denied bool, err error) {
	toolUseID = fmt.Sprintf("toolu_%s_%d", c.Agent, c.seq.Add(1))
	p := claude.PayloadParams{
		ToolName: toolName, ToolInput: input, ToolUseID: toolUseID,
		AgentID: agentID, AgentType: agentType,
	}
	resp, err := c.postHook(c.payload("PreToolUse", p))
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return "", false, fmt.Errorf("PreToolUse post: %s", resp.Status)
	}
	var body struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", false, fmt.Errorf("PreToolUse response: %w", err)
	}
	return toolUseID, body.HookSpecificOutput.PermissionDecision == "deny", nil
}

// PostToolUse posts the Post hook closing a tool call.
func (c *Client) PostToolUse(toolUseID, toolName string, input map[string]any,
	failed bool, agentID, agentType string) error {
	event := "PostToolUse"
	p := claude.PayloadParams{
		ToolName: toolName, ToolInput: input, ToolUseID: toolUseID,
		AgentID: agentID, AgentType: agentType, DurationMs: 3,
		ToolResponse: map[string]any{"scripted": true},
	}
	if failed {
		event = "PostToolUseFailure"
		p.Error = "scripted failure"
	}
	resp, err := c.postHook(c.payload(event, p))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s post: %s", event, resp.Status)
	}
	return nil
}

// HookEvent posts a non-tool hook event verbatim (with optional extras).
func (c *Client) HookEvent(event string, p claude.PayloadParams) error {
	resp, err := c.postHook(c.payload(event, p))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("hook %s post: %s", event, resp.Status)
	}
	return nil
}

// --------------------------------------------------------------------------
// MCP calls (Streamable HTTP, stateless, plain JSON-RPC over POST)
// --------------------------------------------------------------------------

// CallTool issues one tools/call request. The endpoint is stateless: no
// session header exists or is sent (spec/02 §2). initialize is done once
// lazily.
// initMCP performs the one-time initialize handshake at the configured
// (default pinned) protocol revision.
func (c *Client) initMCP() error {
	if c.mcpInit.Load() {
		return nil
	}
	pv := c.ProtocolVersion
	if pv == "" {
		pv = tools.ProtocolVersion
	}
	if err := c.rpc("initialize", map[string]any{
		"protocolVersion": pv,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "scripted", "version": "1.0"},
	}); err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}
	if err := c.rpcNotify("notifications/initialized", map[string]any{}); err != nil {
		return err
	}
	c.mcpInit.Store(true)
	return nil
}

func (c *Client) CallTool(name string, args map[string]any) (map[string]any, error) {
	if err := c.initMCP(); err != nil {
		return nil, err
	}
	res, err := c.rpcResult("tools/call", map[string]any{
		"name": name, "arguments": args,
	})
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(res, &m); err != nil {
		return nil, err
	}
	if isErr, _ := m["isError"].(bool); isErr {
		return m, fmt.Errorf("tool %s returned an error result: %v", name, m["content"])
	}
	return m, nil
}

// ListTools returns the server's tools/list result (C1.6.5).
func (c *Client) ListTools() ([]map[string]any, error) {
	if err := c.initMCP(); err != nil {
		return nil, err
	}
	res, err := c.rpcResult("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

func (c *Client) rpc(method string, params any) error {
	_, err := c.rpcResult(method, params)
	return err
}

// rpcNotify posts a JSON-RPC notification (no id, no result expected).
func (c *Client) rpcNotify(method string, params any) error {
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": method, "params": params,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.MCPURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mcp notify %s: %s", method, resp.Status)
	}
	return nil
}

// rpcResult posts one JSON-RPC request and reads its result, accepting
// either an application/json body or an SSE stream carrying the response.
func (c *Client) rpcResult(method string, params any) (json.RawMessage, error) {
	id := c.mcpID.Add(1)
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	tries := 1 + c.Retries
	var lastErr error
	for i := 0; i < tries; i++ {
		req, err := http.NewRequest("POST", c.MCPURL, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := c.httpClient().Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("mcp %s: %s: %s", method, resp.Status, trunc(body, 200))
			time.Sleep(time.Duration(50*(i+1)) * time.Millisecond)
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("mcp %s: %s: %s", method, resp.Status, trunc(body, 200))
		}
		msg, err := findResponse(id, resp.Header.Get("Content-Type"), body)
		if err != nil {
			return nil, err
		}
		var rpcResp struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(msg, &rpcResp); err != nil {
			return nil, fmt.Errorf("mcp %s: bad response: %w", method, err)
		}
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("mcp %s: rpc error %d: %s",
				method, rpcResp.Error.Code, rpcResp.Error.Message)
		}
		return rpcResp.Result, nil
	}
	return nil, lastErr
}

// findResponse extracts the JSON-RPC reply for id from a plain-JSON or an
// SSE body.
func findResponse(id int64, contentType string, body []byte) (json.RawMessage, error) {
	if strings.Contains(contentType, "text/event-stream") {
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var probe struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal([]byte(data), &probe) == nil && string(probe.ID) == fmt.Sprint(id) {
				return json.RawMessage(data), nil
			}
		}
		return nil, fmt.Errorf("no SSE response for id %d", id)
	}
	return json.RawMessage(body), nil
}

func trunc(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

// --------------------------------------------------------------------------
// Step execution
// --------------------------------------------------------------------------

// runBash executes a command inside the worktree with the ambient
// environment (like the agent's own shell; git inherits GitEnv through the
// process environment below).
func (c *Client) runBash(command string) (int, error) {
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/c"
	}
	cmd := exec.Command(shell, flag, command)
	cmd.Dir = c.Worktree
	cmd.Env = append(os.Environ(), c.GitEnv...)
	out, err := cmd.CombinedOutput()
	c.logf("bash %q -> %v (%d bytes)", command, err, len(out))
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	return -1, err
}

// git runs a git command inside the worktree under the world's env.
func (c *Client) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.Worktree
	env := os.Environ()
	// Strip inherited git overrides so the world config is authoritative.
	var kept []string
	for _, e := range env {
		k := e[:strings.IndexByte(e, '=')]
		if strings.HasPrefix(k, "GIT_CONFIG_") || k == "GIT_DIR" ||
			k == "GIT_WORK_TREE" || k == "GIT_INDEX_FILE" ||
			k == "GIT_OBJECT_DIRECTORY" || k == "GIT_COMMON_DIR" ||
			k == "GIT_CONFIG_NOSYSTEM" || k == "GIT_CEILING_DIRECTORIES" {
			continue
		}
		kept = append(kept, e)
	}
	cmd.Env = append(kept, c.GitEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
