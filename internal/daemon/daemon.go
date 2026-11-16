// Package daemon implements the Court daemon of spec/02-architecture.md
// §2: the only writer of the ledger, bound to a loopback port the OS
// assigns. It serves:
//
//	POST /hook/<token>   agent tool hook payloads -> tool.event
//	POST /mcp/<token>    agent tools over MCP (internal/tools)
//	POST /gate           commit notifications from git hooks -> world.commit
//	POST /admin/*        commission/session registration (bearer token)
//
// The token in the URL path is the identity of a hook post or tool call:
// the current MCP revision is stateless and has no session header, so the
// path is how the daemon attributes an event to an agent (spec/02 §2).
// Subagents the tool starts inside a session share its token; their hook
// payloads carry agent_id and are recorded under a child agent of the
// session's agent (spec/02 §4.1).
package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"court/internal/ledger"
)

// DenyPolicy is evaluated on every PreToolUse hook post. It returns
// ("deny", reason) to refuse the call or ("", "") to abstain and let the
// tool's own permission mode decide. Court never returns "allow".
// worldDir is the session's own world — the exempt subtree of the data
// directory (worlds live under <data-dir>/worlds).
type DenyPolicy interface {
	DecideHook(toolName string, toolInput map[string]any, worldDir string) (string, string)
}

// Options configures a Daemon.
type Options struct {
	// DataDir is the Court data directory for this repository. The daemon
	// writes daemon.json, sessions.json, refusals.log and hooks/post-commit
	// into it.
	DataDir string
	// Policy is the PreToolUse deny evaluator (nil abstains from everything).
	Policy DenyPolicy
	// CourtBin is the court binary the generated git hook script execs.
	// Empty selects os.Executable.
	CourtBin string
	// AdminToken is the bearer token of the /admin endpoints. Empty loads
	// or generates the token recorded in <DataDir>/daemon.json.
	AdminToken string
	// Logger receives diagnostic lines (nil discards).
	Logger *log.Logger
	// AppendDelay, when > 0, sleeps inside the ledger-append critical
	// section before the response is written. It exists so the C1.8.3
	// kill/restart check holds a wide, deterministic window in which a
	// premature acknowledgement would lose a post. The COURT_DAEMON_APPEND_DELAY_MS
	// environment variable sets the same delay for `court daemon`.
	AppendDelay time.Duration
}

// Session is one registered agent session (spec/02 §2).
type Session struct {
	Token      string `json:"token"`
	SessionID  string `json:"session_id"` // the agent tool's own session id, never used for attribution
	Commission string `json:"commission"`
	Agent      string `json:"agent"`
	Parent     string `json:"parent"`
	Tool       string `json:"tool"`
	Version    string `json:"version"`
	World      string `json:"world"`
	Base       string `json:"base"`
	Branch     string `json:"branch"`
	Worktree   string `json:"worktree"`
	Writ       string `json:"writ"`
}

// preEntry is one parked PreToolUse: a call the policy abstained from and
// whose PostToolUse (or matching MCP tool call) has not arrived yet. It
// carries the resolved actor so a subagent's calls attribute correctly
// (spec/02 §5).
type preEntry struct {
	toolUseID string
	tool      string
	digest    string // sha256 of the canonical tool_input, for MCP pairing
	actor     string
	te        *ledger.ToolEvent
	at        time.Time
}

// Daemon is the HTTP endpoint process of spec/02 §2.
type Daemon struct {
	store *ledger.Store
	opts  Options
	ln    net.Listener
	srv   *http.Server
	mux   *http.ServeMux

	mu         sync.Mutex
	sessions   map[string]*Session      // token -> session
	byAgent    map[string]*Session      // commission/agent -> session (children included)
	byTree     map[string]*Session      // normalized worktree path -> session
	started    map[string]bool          // commission/agent already has agent.started
	stopped    map[string]bool          // commission/agent already has agent.stopped
	recorded   map[string]bool          // recorded commit ids
	lastCommit map[string]string        // world -> newest recorded commit
	pending    map[string][]preEntry    // token -> parked PreToolUse entries
	worldMu    map[string]*sync.Mutex   // serialize gate sync per world
	mcpSrv     map[*Session]*mcp.Server // session -> its tools server
	mcpHTTP    http.Handler
	refusalMu  sync.Mutex

	adminToken string
	closed     chan struct{}
	closeOnce  sync.Once
}

// New opens a daemon over the given store. It re-derives its in-memory
// bookkeeping (recorded commits, started/stopped agents) from the ledger so
// a restart loses nothing the ledger has, and backfills request identities
// of the events already appended into the store's idem table so retried
// posts keep deduplicating across restart.
func New(store *ledger.Store, opts Options) (*Daemon, error) {
	if opts.DataDir == "" {
		return nil, fmt.Errorf("daemon: DataDir required")
	}
	d := &Daemon{
		store: store, opts: opts,
		sessions: map[string]*Session{}, byAgent: map[string]*Session{},
		byTree: map[string]*Session{}, started: map[string]bool{},
		stopped:  map[string]bool{},
		recorded: map[string]bool{}, lastCommit: map[string]string{},
		pending: map[string][]preEntry{}, worldMu: map[string]*sync.Mutex{},
		mcpSrv: map[*Session]*mcp.Server{},
		closed: make(chan struct{}),
	}
	d.mcpHTTP = d.newMCPHandler()
	if opts.CourtBin == "" {
		if exe, err := os.Executable(); err == nil {
			opts.CourtBin = exe
		}
	}
	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		return nil, err
	}
	// Sessions load before the ledger rebuild: the idem backfill resolves
	// each recorded event's session so retried requests keep their
	// per-session request identity across restarts.
	if err := d.loadSessions(); err != nil {
		return nil, err
	}
	if err := d.rebuildFromLedger(); err != nil {
		return nil, err
	}
	if err := d.resolveAdminToken(); err != nil {
		return nil, err
	}
	d.mux = http.NewServeMux()
	d.mux.HandleFunc("POST /hook/{token}", d.handleHook)
	d.mux.HandleFunc("POST /mcp/{token}", d.handleMCP)
	d.mux.HandleFunc("POST /gate", d.handleGate)
	d.mux.HandleFunc("POST /admin/commission", d.adminCommission)
	d.mux.HandleFunc("POST /admin/session", d.adminSession)
	d.mux.HandleFunc("GET /admin/info", d.adminInfo)
	d.mux.HandleFunc("POST /admin/shutdown", d.adminShutdown)
	return d, nil
}

// Start binds the loopback listener, regenerates the git hook script for
// the bound port and serves in the background.
func (d *Daemon) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("daemon: bind: %w", err)
	}
	d.ln = ln
	d.srv = &http.Server{
		Handler:           d.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := d.writeGateHook(); err != nil {
		ln.Close()
		return err
	}
	if err := d.writeDaemonFile(); err != nil {
		ln.Close()
		return err
	}
	go func() {
		<-ctx.Done()
		d.Close()
	}()
	go d.srv.Serve(ln)
	go func() {
		<-d.closed
		d.srv.Shutdown(context.Background())
	}()
	return nil
}

// URL is the daemon's base URL (http://127.0.0.1:<port>).
func (d *Daemon) URL() string {
	if d.ln == nil {
		return ""
	}
	return "http://" + d.ln.Addr().String()
}

// Addr returns the bound address.
func (d *Daemon) Addr() string {
	if d.ln == nil {
		return ""
	}
	return d.ln.Addr().String()
}

// AdminToken returns the bearer token of the /admin endpoints.
func (d *Daemon) AdminToken() string { return d.adminToken }

// Close stops the server.
func (d *Daemon) Close() {
	d.closeOnce.Do(func() {
		close(d.closed)
		if d.ln != nil {
			d.ln.Close()
		}
	})
}

func (d *Daemon) logf(format string, a ...any) {
	if d.opts.Logger != nil {
		d.opts.Logger.Printf(format, a...)
	}
}

// --------------------------------------------------------------------------
// Rebuild at (re)start: everything the daemon knows lives in the ledger or
// in sessions.json.
// --------------------------------------------------------------------------

func (d *Daemon) rebuildFromLedger() error {
	events, err := d.store.Events()
	if err != nil {
		return fmt.Errorf("daemon: rebuild: %w", err)
	}
	var idemKeys []string
	for _, ev := range events {
		b, _ := bodyMap(ev.Body)
		switch ev.Type {
		case "agent.started":
			agent := s(b["agent"])
			d.started[ev.Commission+"/"+agent] = true
			idemKeys = append(idemKeys, "agent.started|"+ev.Commission+"/"+agent)
		case "agent.stopped":
			d.stopped[ev.Commission+"/"+s(b["agent"])] = true
		case "commission.opened":
			idemKeys = append(idemKeys, "commission.opened|"+ev.Commission)
		case "world.created":
			idemKeys = append(idemKeys, "world.created|"+s(b["world"]))
		case "world.commit":
			w := s(b["world"])
			c := s(b["commit"])
			d.recorded[c] = true
			d.lastCommit[w] = c
			idemKeys = append(idemKeys, "world.commit|"+c)
		case "tool.event":
			if tuid := s(b["tool_use_id"]); tuid != "" {
				// The key is per session (hookKey), so the rebuild must
				// find the session the event's actor posted under. An
				// event whose session is gone answers no retry anyway —
				// a repost would be refused on the unknown token.
				if sess := d.sessionForActor(ev.Commission, ev.Actor); sess != nil {
					idemKeys = append(idemKeys, hookKey(sess.Token, tuid))
				}
			}
		}
	}
	// Events recorded before the idem table existed still answer their
	// retried requests: the natural keys above re-derive the request
	// identity of everything already in the chain.
	if err := d.store.AddIdemKeys(idemKeys); err != nil {
		return fmt.Errorf("daemon: rebuild idem keys: %w", err)
	}
	return nil
}

// --------------------------------------------------------------------------
// sessions.json persistence
// --------------------------------------------------------------------------

func (d *Daemon) sessionsPath() string { return filepath.Join(d.opts.DataDir, "sessions.json") }
func (d *Daemon) daemonPath() string   { return filepath.Join(d.opts.DataDir, "daemon.json") }
func (d *Daemon) refusalPath() string  { return filepath.Join(d.opts.DataDir, "refusals.log") }

func (d *Daemon) loadSessions() error {
	raw, err := os.ReadFile(d.sessionsPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []*Session
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("daemon: sessions.json: %w", err)
	}
	for _, sess := range list {
		d.register(sess)
	}
	return nil
}

func (d *Daemon) saveSessionsLocked() error {
	list := make([]*Session, 0, len(d.sessions))
	for _, s := range d.sessions {
		list = append(list, s)
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	tmp := d.sessionsPath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, d.sessionsPath())
}

// register indexes a session under mu (caller holds it unless loading).
func (d *Daemon) register(sess *Session) {
	d.sessions[sess.Token] = sess
	d.byAgent[sess.Commission+"/"+sess.Agent] = sess
	if sess.Worktree != "" {
		d.byTree[normPath(sess.Worktree)] = sess
	}
}

// --------------------------------------------------------------------------
// admin token
// --------------------------------------------------------------------------

func (d *Daemon) resolveAdminToken() error {
	if d.opts.AdminToken != "" {
		d.adminToken = d.opts.AdminToken
		return nil
	}
	raw, err := os.ReadFile(d.daemonPath())
	if err == nil {
		var f struct {
			AdminToken string `json:"admin_token"`
		}
		if json.Unmarshal(raw, &f) == nil && f.AdminToken != "" {
			d.adminToken = f.AdminToken
			return nil
		}
	}
	tok, err := randomToken()
	if err != nil {
		return err
	}
	d.adminToken = tok
	return nil
}

func (d *Daemon) writeDaemonFile() error {
	raw, err := json.Marshal(map[string]any{
		"url":         d.URL(),
		"admin_token": d.adminToken,
		"pid":         os.Getpid(),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(d.daemonPath(), raw, 0o600)
}

// --------------------------------------------------------------------------
// Refusals: every refused post is recorded in <DataDir>/refusals.log with
// nothing attributed to an agent (C1.5.2).
// --------------------------------------------------------------------------

func (d *Daemon) refuse(w http.ResponseWriter, r *http.Request, endpoint, reason string, status int) {
	entry := map[string]any{
		"at":       time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"endpoint": endpoint,
		"reason":   reason,
		"remote":   r.RemoteAddr,
	}
	raw, _ := json.Marshal(entry)
	d.refusalMu.Lock()
	f, err := os.OpenFile(d.refusalPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		f.Write(append(raw, '\n'))
		f.Close()
	}
	d.refusalMu.Unlock()
	d.logf("refused %s from %s: %s", endpoint, r.RemoteAddr, reason)
	writeJSON(w, status, map[string]any{"error": reason})
}

func (d *Daemon) sessionOf(r *http.Request) (*Session, bool) {
	token := r.PathValue("token")
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[token]
	return s, ok
}

// sessionForActor resolves the session a recorded event was posted under:
// the actor is agent:<agent> for the session's own calls and
// agent:<agent>:<subagent-id> for a subagent sharing the session's token.
// Longest agent id wins, so a literal ':' inside an agent id still finds
// its own session first.
func (d *Daemon) sessionForActor(commission, actor string) *Session {
	a := strings.TrimPrefix(actor, "agent:")
	for a != "" {
		if sess, ok := d.byAgent[commission+"/"+a]; ok {
			return sess
		}
		i := strings.LastIndexByte(a, ':')
		if i < 0 {
			return nil
		}
		a = a[:i]
	}
	return nil
}

// --------------------------------------------------------------------------
// Append with request-identity idempotency: a retry is the same request
// arriving again, named by its own identity — a hook post's session plus
// tool_use_id, an MCP call's session plus JSON-RPC request id, or a
// client-supplied Idempotency-Key — never the event content. The store
// checks the keys and appends in one transaction, so a retried request is
// recorded once and a distinct request with identical content is recorded
// again; the keys live in the ledger file, so the rule survives a daemon
// restart (C1.8.3).
// --------------------------------------------------------------------------

func (d *Daemon) emit(commission, actor, typ string, body any, idemKeys ...string) (*ledger.Event, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.opts.AppendDelay > 0 {
		time.Sleep(d.opts.AppendDelay)
	}
	// AppendIdem marshals canonically, validates, deduplicates on the
	// request-identity keys and commits in one write transaction; the
	// response goes out only after COMMIT.
	return d.store.AppendIdem(commission, actor, typ, body, idemKeys)
}

// --------------------------------------------------------------------------
// admin endpoints (the only callers are the commissioner/test harness)
// --------------------------------------------------------------------------

func (d *Daemon) adminOK(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") == d.adminToken && d.adminToken != "" {
		return true
	}
	d.refuse(w, r, "admin", "bad admin token", http.StatusUnauthorized)
	return false
}

func (d *Daemon) adminInfo(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	writeJSON(w, 200, map[string]any{"url": d.URL(), "data_dir": d.opts.DataDir})
}

func (d *Daemon) adminShutdown(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	go d.Close()
}

// Commission is the registration payload of /admin/commission.
type Commission struct {
	ID         string         `json:"commission"`
	Goal       string         `json:"goal"`
	DoneChecks []string       `json:"done_checks"`
	Territory  []string       `json:"territory"`
	Budgets    ledger.Budgets `json:"budgets"`
	Attention  int64          `json:"attention"`
	Defaults   []string       `json:"defaults"`
	Exemptions []string       `json:"exemptions"`
}

// OpenCommission records commission.opened. It is idempotent: reopening an
// existing commission is a no-op (the commission id is the request's own
// identity).
func (d *Daemon) OpenCommission(c Commission) error {
	return d.openCommission(c)
}

func (d *Daemon) openCommission(c Commission, extraKeys ...string) error {
	if c.ID == "" || c.Goal == "" {
		return fmt.Errorf("daemon: commission and goal are required")
	}
	_, _, err := d.emit(c.ID, "human", "commission.opened", &ledger.CommissionOpened{
		Goal: c.Goal, DoneChecks: c.DoneChecks, Territory: c.Territory,
		Budgets: c.Budgets, Attention: c.Attention, Defaults: c.Defaults,
		Exemptions: c.Exemptions,
	}, append([]string{"commission.opened|" + c.ID}, extraKeys...)...)
	return err
}

func (d *Daemon) adminCommission(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	var c Commission
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&c); err != nil {
		d.refuse(w, r, "admin", "bad commission body", http.StatusBadRequest)
		return
	}
	if err := d.openCommission(c, clientKey(r, "admin:commission")); err != nil {
		d.refuse(w, r, "admin", err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, 200, map[string]any{"commission": c.ID})
}

// clientKey derives the idempotency key of a request carrying an
// Idempotency-Key header, namespaced per endpoint so one key can never
// cancel a different request. Empty when the header is absent.
func clientKey(r *http.Request, namespace string) string {
	k := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if k == "" {
		return ""
	}
	return "key|" + namespace + "|" + k
}

// StartSession registers an agent session: world.created when the world is
// new, then agent.started; it returns the fresh token and URLs.
func (d *Daemon) StartSession(sess Session) (string, error) {
	return d.startSession(sess)
}

func (d *Daemon) startSession(sess Session, extraKeys ...string) (string, error) {
	if sess.Commission == "" || sess.Agent == "" || sess.World == "" {
		return "", fmt.Errorf("daemon: commission, agent and world are required")
	}
	d.mu.Lock()
	if _, dup := d.byAgent[sess.Commission+"/"+sess.Agent]; dup {
		d.mu.Unlock()
		return "", fmt.Errorf("daemon: agent %s already has a session", sess.Agent)
	}
	if _, known := d.byTree[normPath(sess.Worktree)]; sess.Worktree != "" && known {
		// another session owns this worktree
		for _, other := range d.sessions {
			if normPath(other.Worktree) == normPath(sess.Worktree) && other.World == sess.World {
				d.mu.Unlock()
				return "", fmt.Errorf("daemon: world %s already belongs to agent %s", sess.World, other.Agent)
			}
		}
	}
	if sess.Token == "" {
		tok, err := randomToken()
		if err != nil {
			d.mu.Unlock()
			return "", err
		}
		sess.Token = tok
	}
	if sess.SessionID == "" {
		sid, err := randomToken()
		if err != nil {
			d.mu.Unlock()
			return "", err
		}
		sess.SessionID = sid
	}
	d.register(&sess)
	err := d.saveSessionsLocked()
	d.mu.Unlock()
	if err != nil {
		return "", err
	}
	if sess.Base != "" {
		if _, _, err := d.emit(sess.Commission, "court", "world.created", &ledger.WorldCreated{
			World: sess.World, Base: sess.Base, Branch: sess.Branch},
			append([]string{"world.created|" + sess.World}, extraKeys...)...); err != nil {
			return "", err
		}
	}
	if !d.started[sess.Commission+"/"+sess.Agent] {
		if _, _, err := d.emit(sess.Commission, "court", "agent.started", &ledger.AgentStarted{
			Agent: sess.Agent, Parent: sess.Parent, Tool: sess.Tool,
			Version: sess.Version, World: sess.World, Writ: sess.Writ},
			append([]string{"agent.started|" + sess.Commission + "/" + sess.Agent}, extraKeys...)...); err != nil {
			return "", err
		}
		d.mu.Lock()
		d.started[sess.Commission+"/"+sess.Agent] = true
		d.mu.Unlock()
	}
	return sess.Token, nil
}

func (d *Daemon) adminSession(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	var sess Session
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&sess); err != nil {
		d.refuse(w, r, "admin", "bad session body", http.StatusBadRequest)
		return
	}
	token, err := d.startSession(sess, clientKey(r, "admin:session"))
	if err != nil {
		d.refuse(w, r, "admin", err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, 200, map[string]any{
		"token":    token,
		"hook_url": d.URL() + "/hook/" + token,
		"mcp_url":  d.URL() + "/mcp/" + token,
		"gate_url": d.URL() + "/gate",
	})
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func normPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "marshal", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(raw)
}

func bodyMap(v *ledger.Value) (map[string]*ledger.Value, bool) {
	if v == nil || v.Kind != ledger.Object {
		return nil, false
	}
	out := map[string]*ledger.Value{}
	for _, mem := range v.Obj {
		out[mem.Key] = mem.Val
	}
	return out, true
}

func s(v *ledger.Value) string {
	if v == nil || v.Kind != ledger.String {
		return ""
	}
	return v.Str
}
