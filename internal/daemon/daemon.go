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

	"court/internal/gate"
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
	// Repo is the repository under commission; the spawn tool forks worlds
	// from it (M2.4). Empty disables spawn.
	Repo string
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
	Task       string `json:"task,omitempty"`
	// RequireBackbrief gates this agent's non-read-only tool calls on its
	// backbrief.submitted being in the ledger (M2.2).
	RequireBackbrief bool `json:"require_backbrief,omitempty"`
	// AwaitClerk additionally gates them on the commission's clerk list
	// being recorded (M2.3); only sessions the commissioner opens carry it.
	AwaitClerk bool `json:"await_clerk,omitempty"`
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
	sessions   map[string]*Session // token -> session
	byAgent    map[string]*Session // commission/agent -> session (children included)
	byTree     map[string]*Session // normalized worktree path -> session
	started    map[string]bool     // commission/agent already has agent.started
	stopped    map[string]bool     // commission/agent already has agent.stopped
	recorded   map[string]bool     // recorded commit ids
	lastCommit map[string]string   // world -> newest recorded commit
	// backbriefed marks commission/actor pairs whose backbrief.submitted is
	// in the ledger (the M2.2 ordering gate's release).
	backbriefed map[string]bool
	// writs holds issued writs by id; spent accrues the budgets granted
	// to a writ's children so attenuation is against what remains (§9).
	writs       map[string]*ledger.WritIssued
	writByAgent map[string]string // commission/agent -> its writ id
	spent       map[string]ledger.Budgets
	pending     map[string][]preEntry    // token -> parked PreToolUse entries
	worldMu     map[string]*sync.Mutex   // serialize gate sync per world
	mcpSrv      map[*Session]*mcp.Server // session -> its tools server
	mcpHTTP     http.Handler
	refusalMu   sync.Mutex

	// gateState is the declaration window per world (M3.2), derived from
	// ledger order; gatePasses holds pre-commit authorizations awaiting
	// their commit. syncCh wakes the head scanner that records commits
	// which reached a world without the post-commit hook.
	gateState  *gate.State
	gatePasses map[string]gate.Pass
	syncCh     chan struct{}
	syncMin    time.Duration
	lastSync   time.Time

	adminToken string
	closed     chan struct{}
	closeOnce  sync.Once
	srvDone    chan struct{}
	srvOnce    sync.Once
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
		backbriefed: map[string]bool{},
		writs:       map[string]*ledger.WritIssued{},
		writByAgent: map[string]string{},
		spent:       map[string]ledger.Budgets{},
		pending:     map[string][]preEntry{}, worldMu: map[string]*sync.Mutex{},
		mcpSrv:     map[*Session]*mcp.Server{},
		gateState:  gate.NewState(),
		gatePasses: map[string]gate.Pass{},
		syncCh:     make(chan struct{}, 1),
		syncMin:    200 * time.Millisecond,
		closed:     make(chan struct{}),
		srvDone:    make(chan struct{}),
	}
	d.mcpHTTP = d.newMCPHandler()
	if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
		return nil, err
	}
	// Sessions load before the ledger rebuild: the idem backfill resolves
	// each recorded event's session so retried requests keep their
	// per-session request identity across restarts.
	if err := d.loadSessions(); err != nil {
		return nil, err
	}
	if err := d.loadGatePasses(); err != nil {
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
	d.mux.HandleFunc("POST /gate/pre", d.handleGatePre)
	d.mux.HandleFunc("POST /admin/commission", d.adminCommission)
	d.mux.HandleFunc("POST /admin/session", d.adminSession)
	d.mux.HandleFunc("GET /admin/info", d.adminInfo)
	d.mux.HandleFunc("POST /admin/shutdown", d.adminShutdown)
	d.mux.HandleFunc("POST /admin/clerk", d.adminClerk)
	d.mux.HandleFunc("POST /admin/rule", d.adminRule)
	d.mux.HandleFunc("GET /admin/events", d.adminEvents)
	d.mux.HandleFunc("POST /admin/writ", d.adminWrit)
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
	if err := gate.Install(gate.InstallOptions{
		DataDir: d.opts.DataDir, URL: d.URL() + "/gate", CourtBin: d.opts.CourtBin,
	}); err != nil {
		ln.Close()
		return err
	}
	if err := d.writeDaemonFile(); err != nil {
		ln.Close()
		return err
	}
	go d.syncLoop()
	go func() {
		<-ctx.Done()
		d.Close()
	}()
	go d.srv.Serve(ln)
	go d.watchDaemonFile()
	go func() {
		<-d.closed
		d.srv.Shutdown(context.Background())
		d.srvOnce.Do(func() { close(d.srvDone) })
	}()
	return nil
}

// A daemon whose data directory has been removed can never be found again:
// daemon.json is the only discovery record. Such a process must not run
// forever untracked, so Start watches that file and closes the daemon after
// it has been continuously absent for dataDirGoneLimit.
const (
	dataDirPollInterval = 250 * time.Millisecond
	dataDirGoneLimit    = 5 * time.Second
)

func (d *Daemon) watchDaemonFile() {
	ticker := time.NewTicker(dataDirPollInterval)
	defer ticker.Stop()
	var missingSince time.Time
	for {
		select {
		case <-d.closed:
			return
		case now := <-ticker.C:
			_, err := os.Stat(d.daemonPath())
			if err == nil || !os.IsNotExist(err) {
				missingSince = time.Time{}
				continue
			}
			if missingSince.IsZero() {
				missingSince = now
				continue
			}
			if now.Sub(missingSince) >= dataDirGoneLimit {
				d.logf("data directory record %s is gone; exiting", d.daemonPath())
				d.Close()
				return
			}
		}
	}
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

// Done is closed when Close has been called (or a signal asked to stop).
// It lets the foreground `court daemon` process learn that an
// /admin/shutdown arrived; without it the process would keep running with
// daemon.log and the ledger open after saying it stopped.
func (d *Daemon) Done() <-chan struct{} { return d.closed }

// Wait blocks until the HTTP server has finished shutting down, so the
// process that owns the log and the ledger exits only after the shutdown
// response has been written.
func (d *Daemon) Wait() { <-d.srvDone }

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
		case "backbrief.submitted":
			d.backbriefed[ev.Commission+"/"+ev.Actor] = true
		case "writ.issued":
			var w ledger.WritIssued
			if remarshal(b, &w) == nil {
				d.writs[w.ID] = &w
				d.writByAgent[ev.Commission+"/"+w.Agent] = w.ID
				if pw := d.writs[d.writByAgent[ev.Commission+"/"+w.Parent]]; pw != nil {
					s := d.spent[pw.ID]
					s.Tokens += w.Budgets.Tokens
					s.Minutes += w.Budgets.Minutes
					s.Speculation += w.Budgets.Speculation
					d.spent[pw.ID] = s
				}
			}
		case "commission.opened":
			idemKeys = append(idemKeys, "commission.opened|"+ev.Commission)
		case "world.created":
			idemKeys = append(idemKeys, "world.created|"+s(b["world"]))
		case "world.commit":
			w := s(b["world"])
			c := s(b["commit"])
			d.recorded[c] = true
			d.lastCommit[w] = c
			d.gateState.Committed(gate.WorldKey(ev.Commission, w), ev.Seq)
			idemKeys = append(idemKeys, "world.commit|"+c)
		case "decision.declared", "decision.none":
			if sess := d.sessionForActor(ev.Commission, ev.Actor); sess != nil {
				d.gateState.Declared(gate.WorldKey(ev.Commission, sess.World), ev.Seq)
			}
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
		"pid_start":   processStartID(),
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
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessionForActorLocked(commission, actor)
}

// sessionForActorLocked is sessionForActor for callers already holding d.mu.
func (d *Daemon) sessionForActorLocked(commission, actor string) *Session {
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
	ev, dup, err := d.store.AppendIdem(commission, actor, typ, body, idemKeys)
	if err == nil && typ == "backbrief.submitted" {
		d.backbriefed[commission+"/"+actor] = true
	}
	// The declaration window moves with the ledger (M3.2): a declaration
	// opens its world's gate, an observed commit closes it. Attribution
	// uses the session the actor belongs to, so a subagent's declaration
	// counts for the world it shares.
	if err == nil && !dup {
		switch b := body.(type) {
		case *ledger.DecisionDeclared, *ledger.DecisionNone:
			if sess := d.sessionForActorLocked(commission, actor); sess != nil {
				d.gateState.Declared(gate.WorldKey(commission, sess.World), ev.Seq)
			}
		case *ledger.WorldCommit:
			d.gateState.Committed(gate.WorldKey(commission, b.World), ev.Seq)
		}
	}
	// Every new ledger write is a point at which a commit that bypassed the
	// post-commit hook can be discovered (spec/03 §4.5); the scanner runs
	// asynchronously and coalesces bursts.
	if err == nil && !dup {
		d.requestSyncLocked()
	}
	return ev, dup, err
}

// hasBackbrief reports whether the actor's backbrief.submitted is in the
// ledger (actor is the ledger actor: "agent:<name>" or "clerk").
func (d *Daemon) hasBackbrief(commission, actor string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.backbriefed[commission+"/"+actor]
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

// ClerkResult is the /admin/clerk payload: the clerk's expected-decision
// list, or the invalid-output marker when it failed schema validation.
type ClerkResult struct {
	Commission string            `json:"commission"`
	Goal       string            `json:"goal"`
	Expected   []ledger.Expected `json:"expected_decisions"`
	Invalid    bool              `json:"invalid"`
	Output     string            `json:"output"`
}

// RecordClerk stores the clerk's answer as backbrief.submitted under
// actor clerk — before anything reads it (02 §7). It is idempotent per
// commission: the clerk runs once.
func (d *Daemon) RecordClerk(c ClerkResult) error {
	if c.Commission == "" {
		return fmt.Errorf("daemon: commission is required")
	}
	out := c.Output
	if r := []rune(out); len(r) > 2000 {
		out = string(r[:2000])
	}
	_, _, err := d.emit(c.Commission, "clerk", "backbrief.submitted",
		&ledger.BackbriefSubmitted{
			Goal: c.Goal, ExpectedDecisions: c.Expected,
			WillNotTouch: []string{},
			Invalid:      c.Invalid, Output: out},
		"clerk|"+c.Commission)
	return err
}

func (d *Daemon) adminClerk(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	var c ClerkResult
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&c); err != nil {
		d.refuse(w, r, "admin", "bad clerk body", http.StatusBadRequest)
		return
	}
	if err := d.RecordClerk(c); err != nil {
		d.refuse(w, r, "admin", err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// adminRule records a human ruling from the upfront hearing. The body
// is {commission, decision, option}; only an actual answer reaches
// here — skipped questions never emit decision.ruled (C2.5.1).
func (d *Daemon) adminRule(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	var req struct {
		Commission string `json:"commission"`
		Decision   string `json:"decision"`
		Option     string `json:"option"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		d.refuse(w, r, "admin", "bad rule body", http.StatusBadRequest)
		return
	}
	if req.Commission == "" || req.Decision == "" || req.Option == "" {
		d.refuse(w, r, "admin", "rule needs commission, decision and option",
			http.StatusBadRequest)
		return
	}
	if _, _, err := d.emit(req.Commission, "human", "decision.ruled",
		&ledger.DecisionRuled{Decision: req.Decision, Option: req.Option},
		"rule|"+req.Commission+"/"+req.Decision+"/"+req.Option); err != nil {
		d.refuse(w, r, "admin", err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// adminEvents returns the commission's events in ledger order (or all
// events without the filter), for the commissioner and checks.
func (d *Daemon) adminEvents(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	comm := r.URL.Query().Get("commission")
	evs, err := d.store.Events()
	if err != nil {
		d.refuse(w, r, "admin", err.Error(), http.StatusInternalServerError)
		return
	}
	var out []*ledger.Event
	for _, e := range evs {
		if comm == "" || e.Commission == comm {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []*ledger.Event{}
	}
	writeJSON(w, 200, map[string]any{"events": out})
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
