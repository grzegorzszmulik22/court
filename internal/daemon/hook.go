package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"court/internal/ledger"
)

// This file maps hook posts to tool.event bodies per spec/02 §4.1 and
// keeps only the selected input fields of spec/01 §11.3: file paths, the
// command line's first 500 characters, and host names. The mapping table
// (inputKeys) is the only tool-specific knowledge the endpoint has, per
// spec/02 §4.3.

// inputKeys maps a tool name to the tool_input members holding a file path
// (single string or list), the command line, and a host name. Unknown
// tools contribute no selected fields; the event still records tool name,
// digest and outcome.
var inputKeys = map[string]struct {
	paths   []string // string path members
	pathsLs []string // array-of-string path members
	command string
	hosts   []string
}{
	"Read":         {paths: []string{"file_path"}},
	"Write":        {paths: []string{"file_path"}},
	"Edit":         {paths: []string{"file_path"}},
	"MultiEdit":    {paths: []string{"file_path"}},
	"NotebookEdit": {paths: []string{"notebook_path"}},
	"Glob":         {paths: []string{"path"}},
	"Grep":         {paths: []string{"path"}},
	"LS":           {paths: []string{"path"}},
	"Bash":         {command: "command"},
	"WebFetch":     {hosts: []string{"url"}},
	"WebSearch":    {},
	"Agent":        {},
	"Task":         {},
}

// hookPayload is one decoded hook post. Fields are raw so nothing the
// mapping does not select is ever retained.
type hookPayload struct {
	HookEvent   string         `json:"hook_event_name"`
	SessionID   string         `json:"session_id"`
	ToolName    string         `json:"tool_name"`
	ToolUseID   string         `json:"tool_use_id"`
	ToolInput   map[string]any `json:"tool_input"`
	AgentID     string         `json:"agent_id"`
	AgentType   string         `json:"agent_type"`
	MCPServer   map[string]any `json:"mcp_server"`
	RawBody     []byte         `json:"-"`
}

// inputDigest hashes tool_input canonically; it both fills
// ToolEvent.InputDigest and pairs an MCP call with its PreToolUse (the
// arguments are the same bytes semantically).
func inputDigest(toolInput map[string]any) (string, error) {
	raw, err := json.Marshal(toolInput)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// handleHook serves POST /hook/<token>.
func (d *Daemon) handleHook(w http.ResponseWriter, r *http.Request) {
	var p hookPayload
	body := http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(body).Decode(&p); err != nil {
		d.refuse(w, r, "hook", "payload is not a hook object", http.StatusBadRequest)
		return
	}
	var sess *Session
	var ok bool
	d.mu.Lock()
	for _, s := range d.sessions {
		if s.SessionID == p.SessionID {
			sess, ok = s, true
			break
		}
	}
	d.mu.Unlock()
	if !ok {
		d.refuse(w, r, "hook", "unknown token", http.StatusNotFound)
		return
	}
	switch p.HookEvent {
	case "PreToolUse":
		d.onPreToolUse(w, r, sess, &p)
	case "PostToolUse", "PostToolUseFailure":
		d.onPostToolUse(w, r, sess, &p, p.HookEvent == "PostToolUseFailure")
	default:
		// SessionStart, Stop, SubagentStart/Stop, UserPromptSubmit and the
		// rest are acknowledged without a ledger event: they carry prose and
		// transcripts, and only tool use is recorded (spec/02 §4.1).
		writeJSON(w, 200, map[string]any{})
	}
}

// actorOf resolves the ledger actor of a hook post: a payload carrying
// agent_id comes from a subagent inside the session (research §1a) and is
// attributed to the child agent; anything else is the session's agent.
func (d *Daemon) actorOf(sess *Session, p *hookPayload) (string, error) {
	if p.AgentID == "" {
		return sess.Agent, nil
	}
	child := sess.Agent + ":" + p.AgentID
	key := sess.Commission + "/" + child
	d.mu.Lock()
	known := d.started[key]
	d.mu.Unlock()
	if !known {
		if _, _, err := d.emit(sess.Commission, "court", "agent.started",
			&ledger.AgentStarted{
				Agent: child, Parent: sess.Agent, Tool: sess.Tool,
				Version: sess.Version, World: sess.World, Writ: sess.Writ}); err != nil {
			return "", err
		}
		d.mu.Lock()
		d.started[key] = true
		d.mu.Unlock()
	}
	return child, nil
}

// buildToolEvent derives the stored body: digest of the full tool_input,
// selected fields only.
func buildToolEvent(p *hookPayload, outcome, reason string) (*ledger.ToolEvent, error) {
	digest, err := inputDigest(p.ToolInput)
	if err != nil {
		return nil, err
	}
	te := &ledger.ToolEvent{
		Tool:        p.ToolName,
		ToolUseID:   p.ToolUseID,
		InputDigest: digest,
		Outcome:     outcome,
		Reason:      reason,
	}
	keys, known := inputKeys[p.ToolName]
	if !known && strings.HasPrefix(p.ToolName, "mcp__") {
		// MCP tools carry arbitrary input; Court's own are matched by the
		// mcp__court__ prefix and select nothing but the call itself.
		known = true
	}
	if known {
		for _, k := range keys.paths {
			if v, ok := p.ToolInput[k].(string); ok && v != "" {
				te.Fields.Paths = append(te.Fields.Paths, v)
			}
		}
		for _, k := range keys.pathsLs {
			if arr, ok := p.ToolInput[k].([]any); ok {
				for _, it := range arr {
					if v, ok := it.(string); ok && v != "" {
						te.Fields.Paths = append(te.Fields.Paths, v)
					}
				}
			}
		}
		if keys.command != "" {
			if v, ok := p.ToolInput[keys.command].(string); ok {
				if len(v) > 500 {
					v = v[:500]
				}
				te.Fields.Command = v
			}
		}
		for _, k := range keys.hosts {
			if v, ok := p.ToolInput[k].(string); ok {
				if h := hostOf(v); h != "" {
					te.Fields.Hosts = append(te.Fields.Hosts, h)
				}
			}
		}
	}
	return te, nil
}

// hostOf extracts the host name of a URL string (no path, no query).
func hostOf(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if h, _, err := net.SplitHostPort(u); err == nil {
		return h
	}
	return u
}

// onPreToolUse runs the deny policy. A denied call never runs and no
// PostToolUse follows, so its tool.event is written now with the reason;
// an abstained call is parked until its Post arrives.
func (d *Daemon) onPreToolUse(w http.ResponseWriter, r *http.Request,
	sess *Session, p *hookPayload) {
	if p.ToolName == "" {
		d.refuse(w, r, "hook", "PreToolUse without tool_name", http.StatusBadRequest)
		return
	}
	actor, err := d.actorOf(sess, p)
	if err != nil {
		d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
		return
	}
	decision, reason := "", ""
	if d.opts.Policy != nil {
		decision, reason = d.opts.Policy.DecideHook(p.ToolName, p.ToolInput, sess.Worktree)
	}
	if decision == "deny" {
		te, err := buildToolEvent(p, "denied", reason)
		if err != nil {
			d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
			return
		}
		if _, _, err := d.emit(sess.Commission, "agent:"+actor, "tool.event", te); err != nil {
			d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, 200, map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": reason,
			},
		})
		return
	}
	te, err := buildToolEvent(p, "ok", "")
	if err != nil {
		d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
		return
	}
	d.mu.Lock()
	d.park(sess.Token, preEntry{
		toolUseID: p.ToolUseID, tool: p.ToolName, digest: te.InputDigest,
		actor: actor, te: te, at: time.Now(),
	})
	d.mu.Unlock()
	writeJSON(w, 200, map[string]any{})
}

// onPostToolUse completes a parked PreToolUse into one tool.event.
func (d *Daemon) onPostToolUse(w http.ResponseWriter, r *http.Request,
	sess *Session, p *hookPayload, failed bool) {
	if p.ToolName == "" {
		d.refuse(w, r, "hook", "PostToolUse without tool_name", http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	pre, had := d.takeByTUID(sess.Token, p.ToolUseID)
	d.mu.Unlock()
	var actor string
	var te *ledger.ToolEvent
	var err error
	if had {
		actor, te = pre.actor, pre.te
	} else {
		// A Post with no parked Pre (daemon restarted mid-call, or the pre
		// was refused before parking) still records, under the agent the
		// payload itself names.
		actor, err = d.actorOf(sess, p)
		if err != nil {
			d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
			return
		}
		te, err = buildToolEvent(p, "ok", "")
		if err != nil {
			d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if failed {
		te.Outcome = "error"
	}
	if _, _, err := d.emit(sess.Commission, "agent:"+actor, "tool.event", te); err != nil {
		d.refuse(w, r, "hook", err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, 200, map[string]any{})
}

// park appends a parked PreToolUse entry (caller holds d.mu).
func (d *Daemon) park(token string, e preEntry) {
	d.pending[token] = append(d.pending[token], e)
}

// takeByTUID removes the parked entry for tool_use_id (caller holds d.mu).
func (d *Daemon) takeByTUID(token, toolUseID string) (preEntry, bool) {
	list := d.pending[token]
	for i, e := range list {
		if e.toolUseID == toolUseID {
			d.pending[token] = append(list[:i], list[i+1:]...)
			return e, true
		}
	}
	return preEntry{}, false
}

// matchMCP finds the oldest parked entry for an MCP tool call with these
// arguments: the pairing rule of spec/02 §5. It does not consume the entry;
// the matching PostToolUse (or a later cleanup) does, since the MCP call
// itself emits no tool.event.
func (d *Daemon) matchMCP(token, tool string, argsDigest string) (preEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.pending[token] {
		if e.tool == tool && e.digest == argsDigest {
			return e, true
		}
	}
	return preEntry{}, false
}
