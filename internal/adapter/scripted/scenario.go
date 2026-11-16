package scripted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"court/internal/adapter/claude"
	"court/internal/daemon"
	"court/internal/ledger"
	"court/internal/world"
)

// Harness is a complete in-process commission for hermetic checks: a
// daemon over a real ledger and real git worlds, with scripted agents
// registered through the admin API. It lives in the scripted adapter
// package (not _test.go) because every chunk's checks drive it — checks
// run `go run`/`go test` and share the same machinery.
type Harness struct {
	Repo       string
	DataDir    string
	Commission string
	Store      *ledger.Store
	Daemon     *daemon.Daemon
	Root       *world.GitWorld
	AdminToken string
	CourtBin   string // path used by the generated post-commit hook

	sessions map[string]*Client // agent id -> client
	rnd      *rand.Rand
	mu       sync.Mutex
}

// Options configures a Harness.
type Options struct {
	// Repo is the repository under commission (must exist, git-initialized,
	// with at least one commit).
	Repo string
	// DataDir is the Court data directory (outside the repo).
	DataDir string
	// Commission id.
	Commission string
	// CourtBin is the court binary the gate hook execs; defaults to
	// $COURT_BIN or a build under DataDir.
	CourtBin string
	// Seed seeds duplicate-delivery randomness (0 = never duplicate).
	Seed int64
	// AppendDelay widens the commit window for kill tests.
	AppendDelay time.Duration
	// Policy overrides the deny policy (nil = claude.Policy{DataDir}).
	Policy daemon.DenyPolicy
}

// NewHarness wires the store, world root and daemon.
func NewHarness(o Options) (*Harness, error) {
	if o.Repo == "" || o.DataDir == "" || o.Commission == "" {
		return nil, fmt.Errorf("harness: Repo, DataDir and Commission are required")
	}
	if err := os.MkdirAll(o.DataDir, 0o755); err != nil {
		return nil, err
	}
	store, err := ledger.Open(filepath.Join(o.DataDir, "ledger.db"))
	if err != nil {
		return nil, err
	}
	root, err := world.Open(o.Repo, o.DataDir, o.Commission)
	if err != nil {
		store.Close()
		return nil, err
	}
	bin := o.CourtBin
	if bin == "" {
		bin = os.Getenv("COURT_BIN")
	}
	pol := o.Policy
	if pol == nil {
		pol = claude.Policy{DataDir: o.DataDir}
	}
	d, err := daemon.New(store, daemon.Options{
		DataDir: o.DataDir,
		Policy:  pol, CourtBin: bin,
		AppendDelay: o.AppendDelay,
		Repo:        o.Repo,
	})
	if err != nil {
		store.Close()
		return nil, err
	}
	h := &Harness{
		Repo: o.Repo, DataDir: o.DataDir, Commission: o.Commission,
		Store: store, Daemon: d, Root: root, CourtBin: bin,
		sessions: map[string]*Client{},
	}
	if o.Seed != 0 {
		h.rnd = rand.New(rand.NewSource(o.Seed))
	}
	return h, nil
}

// Start binds the listener (OS-chosen loopback port).
func (h *Harness) Start() error { return h.Daemon.Start(context.Background()) }

// Close releases everything; the ledger stays on disk.
func (h *Harness) Close() {
	h.Daemon.Close()
	h.Store.Close()
}

// OpenCommission posts /admin/commission.
func (h *Harness) OpenCommission(goal string, doneChecks []string) error {
	return h.postJSON("/admin/commission", map[string]any{
		"commission": h.Commission, "goal": goal,
		"done_checks": doneChecks, "territory": []string{},
		"budgets":   map[string]any{"tokens": 0, "minutes": 0, "speculation": 0},
		"attention": 0, "defaults": []string{}, "exemptions": []string{},
	})
}

// AddAgent forks a world and registers a session for the agent, returning
// its ready client.
func (h *Harness) AddAgent(spec *AgentSpec) (*Client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.sessions[spec.ID]; dup {
		return nil, fmt.Errorf("harness: agent %s already exists", spec.ID)
	}
	base, err := h.head()
	if err != nil {
		return nil, err
	}
	w, err := h.Root.Fork(world.Commit(base), spec.ID)
	if err != nil {
		return nil, err
	}
	gw, ok := w.(*world.GitWorld)
	if !ok {
		return nil, fmt.Errorf("harness: unexpected world type")
	}
	parent := spec.Parent
	if parent == "" {
		parent = "human"
	}
	sid := newSessionID()
	var resp struct {
		Token   string `json:"token"`
		HookURL string `json:"hook_url"`
		MCPURL  string `json:"mcp_url"`
	}
	if err := h.postJSONInto("/admin/session", map[string]any{
		"commission": h.Commission, "agent": spec.ID, "parent": parent,
		"tool": "scripted", "version": "2.1.325", "session_id": sid,
		"world": spec.ID, "base": base, "branch": gw.Branch(),
		"worktree": gw.Path(), "writ": spec.Writ,
		"require_backbrief": spec.RequireBackbrief,
		"await_clerk":       spec.AwaitClerk,
	}, &resp); err != nil {
		return nil, err
	}
	c := &Client{
		HookURL: resp.HookURL, MCPURL: resp.MCPURL,
		Worktree: gw.Path(), Agent: spec.ID,
		SessionID:  sid,
		PromptID:   fmt.Sprintf("prompt-%s-%s", spec.ID, sid),
		Transcript: filepath.Join(h.DataDir, "transcripts", spec.ID+".jsonl"),
		GitEnv:     gw.GitEnv(),
		Retries:    3,
	}
	if h.rnd != nil {
		c.Rnd = rand.New(rand.NewSource(h.rnd.Int63()))
	}
	h.sessions[spec.ID] = c
	return c, nil
}

// Client returns a registered agent's client.
func (h *Harness) Client(agent string) (*Client, bool) {
	c, ok := h.sessions[agent]
	return c, ok
}

// head resolves the repository's current HEAD.
func (h *Harness) head() (string, error) {
	out, err := gitOutput(h.Repo, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

// newSessionID makes a tool-side session id for a scripted agent (the
// daemon binds it to the token; every payload must carry it back).
var sidCounter = struct {
	mu sync.Mutex
	n  int64
}{}

func newSessionID() string {
	sidCounter.mu.Lock()
	defer sidCounter.mu.Unlock()
	sidCounter.n++
	return fmt.Sprintf("sess-%d-%d", time.Now().UnixNano(), sidCounter.n)
}

// RunScript registers every agent and runs its steps. Concurrency belongs
// to the caller (agents run sequentially here so step order is script
// order).
func (h *Harness) RunScript(sc *Script) error {
	for _, a := range sc.Agents {
		c, err := h.AddAgent(a)
		if err != nil {
			return err
		}
		if err := c.Run(a.Steps); err != nil {
			return fmt.Errorf("agent %s: %w", a.ID, err)
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// admin HTTP helpers
// --------------------------------------------------------------------------

func (h *Harness) postJSON(path string, v any) error {
	return h.postJSONInto(path, v, nil)
}

func (h *Harness) postJSONInto(path string, v any, into any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", h.Daemon.URL()+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.Daemon.AdminToken())
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	if into != nil {
		return json.Unmarshal(body, into)
	}
	return nil
}

// Events returns the whole ledger in order.
func (h *Harness) Events() ([]*ledger.Event, error) { return h.Store.Events() }

// IssueWrit posts a writ to /admin/writ and returns its id.
func (h *Harness) IssueWrit(commission string, w *ledger.WritIssued) (string, error) {
	var out struct {
		Writ string `json:"writ"`
	}
	if err := h.postJSONInto("/admin/writ", map[string]any{
		"commission": commission, "writ": w}, &out); err != nil {
		return "", err
	}
	return out.Writ, nil
}

// OpenCommissionID opens another commission on the same daemon. Checks
// that need hundreds of commissions share the harness.
func (h *Harness) OpenCommissionID(id, goal string, doneChecks []string) error {
	return h.postJSON("/admin/commission", map[string]any{
		"commission": id, "goal": goal,
		"done_checks": doneChecks, "territory": []string{},
		"budgets":   map[string]any{"tokens": 0, "minutes": 0, "speculation": 0},
		"attention": 0, "defaults": []string{}, "exemptions": []string{},
	})
}

// AddSession registers a session without forking a world: the ordering
// checks need hundreds of sessions and a real worktree per agent only
// slows them down. The session's world id is the agent's plus a suffix
// so the byAgent key stays unique inside its commission.
func (h *Harness) AddSession(commission string, spec *AgentSpec) (*Client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	parent := spec.Parent
	if parent == "" {
		parent = "human"
	}
	sid := newSessionID()
	var resp struct {
		Token   string `json:"token"`
		HookURL string `json:"hook_url"`
		MCPURL  string `json:"mcp_url"`
	}
	if err := h.postJSONInto("/admin/session", map[string]any{
		"commission": commission, "agent": spec.ID, "parent": parent,
		"tool": "scripted", "version": "2.1.325", "session_id": sid,
		"world":             spec.ID + "-world",
		"worktree":          filepath.Join(h.DataDir, "noworld", commission+"-"+spec.ID),
		"require_backbrief": spec.RequireBackbrief,
		"await_clerk":       spec.AwaitClerk,
		"writ":              spec.Writ,
	}, &resp); err != nil {
		return nil, err
	}
	c := &Client{
		HookURL: resp.HookURL, MCPURL: resp.MCPURL,
		Worktree:  filepath.Join(h.DataDir, "noworld", commission+"-"+spec.ID),
		Agent:     spec.ID,
		SessionID: sid,
		PromptID:  fmt.Sprintf("prompt-%s-%s", spec.ID, sid),
		Retries:   3,
	}
	h.sessions[commission+"/"+spec.ID] = c
	return c, nil
}

// Refusals returns the raw refusals.log lines.
func (h *Harness) Refusals() ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(h.DataDir, "refusals.log"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

// --------------------------------------------------------------------------
// fixture repository helper
// --------------------------------------------------------------------------

// InitRepo creates a small git repository at dir with one commit and the
// given files. Used by checks to build the commissioned repository.
func InitRepo(dir string, files map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return err
		}
	}
	if err := git(dir, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := git(dir, "add", "-A"); err != nil {
		return err
	}
	return git(dir, "-c", "user.email=court@local", "-c", "user.name=court",
		"commit", "-qm", "initial")
}

func git(dir string, args ...string) error {
	out, err := gitOutput(dir, args...)
	if err != nil {
		return fmt.Errorf("git %s: %w (%s)", args[0], err, out)
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// --------------------------------------------------------------------------
// expected-event projection for C1.5.1/C1.8.1
// --------------------------------------------------------------------------

// ExpectedEvent is what a script says should reach the ledger, per step,
// before any event is read back (C1.8.1: compared to scripts, not to
// Court's counters).
type ExpectedEvent struct {
	Agent string `json:"agent"`
	Type  string `json:"type"` // tool.event | world.commit | decision.declared | ...
	Tool  string `json:"tool,omitempty"`
	Step  int    `json:"step"`
}

// Expected projects a script's steps into the events they must produce.
// A denied call still records one tool.event, so every tool-shaped step
// produces exactly one; a commit produces exactly one world.commit.
func Expected(sc *Script) []ExpectedEvent {
	var out []ExpectedEvent
	for _, a := range sc.Agents {
		for i, st := range a.Steps {
			switch st.Kind() {
			case "edit":
				out = append(out, ExpectedEvent{a.ID, "tool.event", "Edit", i})
			case "tool":
				out = append(out, ExpectedEvent{a.ID, "tool.event", st.Tool.Name, i})
			case "commit":
				out = append(out, ExpectedEvent{a.ID, "world.commit", "", i})
			case "declare":
				out = append(out, ExpectedEvent{a.ID, "tool.event", "mcp__court__record_decision", i})
				out = append(out, ExpectedEvent{a.ID, "decision.declared", "", i})
			case "alarm":
				out = append(out, ExpectedEvent{a.ID, "tool.event", "mcp__court__alarm", i})
				out = append(out, ExpectedEvent{a.ID, "alarm.raised", "", i})
			case "claim_done":
				out = append(out, ExpectedEvent{a.ID, "tool.event", "mcp__court__claim_done", i})
				out = append(out, ExpectedEvent{a.ID, "claim.done", "", i})
			case "no_decisions":
				out = append(out, ExpectedEvent{a.ID, "tool.event", "mcp__court__no_decisions", i})
				out = append(out, ExpectedEvent{a.ID, "decision.none", "", i})
			case "backbrief":
				out = append(out, ExpectedEvent{a.ID, "tool.event", "mcp__court__backbrief", i})
				out = append(out, ExpectedEvent{a.ID, "backbrief.submitted", "", i})
			case "hook":
				if isToolEvent(st.Hook.Event) {
					out = append(out, ExpectedEvent{a.ID, "tool.event", st.Hook.ToolName, i})
				}
			}
		}
	}
	return out
}

// isToolEvent reports whether a hook event produces a tool.event:
// PostToolUse/Failure produce the stored event (Pre is parked); a denied
// Pre produces it at once — the harness cannot see the decision from the
// script, so raw hook steps stay accountable through their Post.
func isToolEvent(event string) bool {
	return event == "PostToolUse" || event == "PostToolUseFailure"
}
