package commission

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"court/internal/daemon"
	"court/internal/ledger"
	"court/internal/world"
)

// Orchestrator is the fixed agent id of the commission's orchestrator.
const Orchestrator = "orchestrator"

// Options carries the parsed `court commission` flags. Given names the
// flags the human actually supplied — everything else is a default and is
// recorded as such in commission.opened.defaults.
type Options struct {
	Dir       string   // where to look for the repository ("" = cwd)
	Goal      string   // the commission's goal (positional)
	Done      []string // --done, repeatable
	Territory []string // --territory, repeatable
	Tokens    int64    // --tokens
	Minutes   int64    // --minutes
	Attention int64    // --attention
	Given     map[string]bool
	DataDir   string // --data-dir
	Script    string // --script: scripted-orchestrator fixture (checks)
	Clerk     string // --clerk: recorded clerk output file
	ClerkCmd  string // --clerk-cmd: command producing the clerk output
	In        io.Reader
	Out       io.Writer
	CourtBin  string
}

// promptText is the commission text handed to the orchestrator (spec/02
// §5: goal, territory, done-checks and the declaration sentence).
func promptText(goal string, territory, done []string) string {
	return fmt.Sprintf("Commission: %s\nTerritory: %s\nDone-checks: %s\n"+
		"Submit your backbrief first: restate the goal, give the plan "+
		"(files, estimated lines) and the decisions you expect to face. "+
		"Declare each decision you make with record_decision before you commit.",
		goal, jsonList(territory), jsonList(done))
}

func jsonList(v []string) string {
	if len(v) == 0 {
		return "(none)"
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

// AgentSession is what `court agent` reads to attach to its session.
type AgentSession struct {
	Adapter    string   `json:"adapter"`
	Agent      string   `json:"agent"`
	Commission string   `json:"commission"`
	HookURL    string   `json:"hook_url"`
	MCPURL     string   `json:"mcp_url"`
	SessionID  string   `json:"session_id"`
	PromptID   string   `json:"prompt_id"`
	Transcript string   `json:"transcript"`
	Worktree   string   `json:"worktree"`
	GitEnv     []string `json:"git_env"`
	Script     string   `json:"script,omitempty"`
	Prompt     string   `json:"prompt"`
	DataDir    string   `json:"data_dir"`
	Token      string   `json:"token"`
	World      string   `json:"world"`
	Writ       string   `json:"writ"`
	Tool       string   `json:"tool"`
	Version    string   `json:"version"`
}

// Run executes `court commission`. Everything the human did not supply is
// a recorded default; work begins the moment the orchestrator session
// exists — the command then watches the backbrief phase and, on a
// terminal, holds the upfront hearing of M2.5.
func Run(o *Options) error {
	if o == nil {
		return fmt.Errorf("options required")
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Given == nil {
		o.Given = map[string]bool{}
	}
	dir := o.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	repo, err := RepoRoot(dir)
	if err != nil {
		return fmt.Errorf("court commission must be run inside a git repository.")
	}
	bin := o.CourtBin
	if bin == "" {
		bin = os.Getenv("COURT_BIN")
	}
	if bin == "" {
		bin, err = os.Executable()
		if err != nil {
			return err
		}
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return err
	}

	// Resolve every commission value, tracking which were defaults.
	var defaults []string
	given := func(name string) bool { return o.Given[name] }
	doneChecks := o.Done
	var rules []string
	if !given("done") {
		doneChecks, rules = DetectDoneChecks(repo)
		defaults = append(defaults, "done-checks")
	}
	territory := o.Territory
	if !given("territory") {
		territory = append([]string(nil), Defaults.Territory...)
		defaults = append(defaults, "territory")
	}
	budgets := ledger.Budgets{Tokens: o.Tokens, Minutes: o.Minutes,
		Speculation: Defaults.Speculation}
	if !given("tokens") {
		budgets.Tokens = Defaults.Tokens
		defaults = append(defaults, "tokens")
	}
	if !given("minutes") {
		budgets.Minutes = Defaults.Minutes
		defaults = append(defaults, "minutes")
	}
	// speculation has no flag: the built-in value is always a default.
	defaults = append(defaults, "speculation")
	attention := o.Attention
	if !given("attention") {
		attention = Defaults.Attention
		defaults = append(defaults, "attention")
	}

	id, err := CommissionID()
	if err != nil {
		return err
	}
	dataDir, err := DataDir(o.DataDir, repo)
	if err != nil {
		return err
	}

	cl, err := EnsureDaemon(bin, dataDir, repo)
	if err != nil {
		return err
	}
	if err := cl.OpenCommission(daemon.Commission{
		ID: id, Goal: o.Goal, DoneChecks: doneChecks, Territory: territory,
		Budgets: budgets, Attention: attention, Defaults: defaults,
		Exemptions: []string{},
	}); err != nil {
		return err
	}

	// The orchestrator's world and session. The world id is unique per
	// commission: data directories are per repository, so repeated
	// commissions share the worlds/ root.
	worldName := Orchestrator + "-" + id
	root, err := world.Open(repo, dataDir, id)
	if err != nil {
		return err
	}
	head, err := repoHead(repo)
	if err != nil {
		return err
	}
	w, err := root.Fork(world.Commit(head), worldName)
	if err != nil {
		return err
	}
	gw, ok := w.(*world.GitWorld)
	if !ok {
		return fmt.Errorf("unexpected world backend")
	}
	tool, version := "claude-code", ""
	if o.Script != "" {
		tool = "scripted"
	}
	sid, err := CommissionID()
	if err != nil {
		return err
	}
	sid = "sess-" + sid
	// The clerk gate holds the orchestrator's first write until the
	// clerk's list is in; with no clerk configured there is nothing to
	// wait for (C2.3.4).
	clerkConfigured := o.Clerk != "" || o.ClerkCmd != ""
	sess, err := cl.StartSession(daemon.Session{
		Commission: id, Agent: Orchestrator, Parent: "human",
		Tool: tool, Version: version, SessionID: sid,
		World: worldName, Base: head, Branch: gw.Branch(),
		Worktree: gw.Path(), Writ: "",
		RequireBackbrief: true, AwaitClerk: clerkConfigured,
	})
	if err != nil {
		return err
	}

	// Hand the session to a detached `court agent` process. The agent
	// state dir is commission-unique, like the world.
	agDir := filepath.Join(dataDir, "agents", Orchestrator+"-"+id)
	if err := os.MkdirAll(agDir, 0o755); err != nil {
		return err
	}
	adapter := "claude"
	if o.Script != "" {
		adapter = "scripted"
	}
	sf := AgentSession{
		Adapter: adapter, Agent: Orchestrator, Commission: id,
		HookURL: sess.HookURL, MCPURL: sess.MCPURL,
		SessionID:  sid,
		PromptID:   "prompt-" + Orchestrator,
		Transcript: filepath.Join(agDir, "transcript.jsonl"),
		Worktree:   gw.Path(), GitEnv: gw.GitEnv(),
		Script:  o.Script,
		Prompt:  promptText(o.Goal, territory, doneChecks),
		DataDir: dataDir, Token: sess.Token,
		World: worldName, Writ: "",
		Tool: tool, Version: version,
	}
	raw, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	sessPath := filepath.Join(agDir, "session.json")
	if err := os.WriteFile(sessPath, raw, 0o600); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(agDir, "agent.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, "agent", "--session", sessPath)
	cmd.Dir = gw.Path()
	cmd.Stdout, cmd.Stderr = logf, logf
	detach(cmd)
	if err := cmd.Start(); err != nil {
		logf.Close()
		return fmt.Errorf("agent start: %w", err)
	}
	go cmd.Wait()
	logf.Close()

	// What was used, one line per resolution rule (M2.1 output).
	fmt.Fprintf(o.Out, "commission %s opened: %q\n", id, o.Goal)
	if len(doneChecks) == 0 {
		fmt.Fprintln(o.Out,
			"no test command detected: done-claims cannot be checked")
	} else {
		for i, c := range doneChecks {
			rule := "flag"
			if i < len(rules) {
				rule = rules[i]
			}
			fmt.Fprintf(o.Out, "done-check: %s (rule: %s)\n", c, rule)
		}
	}
	fmt.Fprintf(o.Out, "territory: %s\n", jsonList(territory))
	fmt.Fprintf(o.Out, "budgets: tokens=%d minutes=%d speculation=%d attention=%d min/day\n",
		budgets.Tokens, budgets.Minutes, budgets.Speculation, attention)
	fmt.Fprintf(o.Out, "orchestrator: world %s on %s (session %s)\n",
		gw.Path(), gw.Branch(), sess.Token[:8])

	// M2.3: the blind clerk sees only the commission; its output enters
	// the ledger as the clerk's backbrief.submitted before use.
	if clerkConfigured {
		_, reqRaw, err := WriteClerkRequest(agDir, id, o.Goal)
		if err != nil {
			return err
		}
		raw, err := RunClerk(o, reqRaw)
		if err != nil {
			return err
		}
		exp, perr := ParseClerkOutput(raw)
		if perr != nil {
			if err := cl.Clerk(daemon.ClerkResult{
				Commission: id, Goal: o.Goal,
				Expected: []ledger.Expected{},
				Invalid:  true, Output: raw}); err != nil {
				return err
			}
			fmt.Fprintln(o.Out,
				"clerk output was invalid: proceeding on the orchestrator's list alone")
		} else {
			if err := cl.Clerk(daemon.ClerkResult{
				Commission: id, Goal: o.Goal, Expected: exp}); err != nil {
				return err
			}
		}
	} else {
		fmt.Fprintln(o.Out,
			"no clerk model configured: proceeding on the orchestrator's list alone")
	}

	// Later phases (comparison, hearing) run here.
	return nil
}

// repoHead resolves HEAD of the repository.
func repoHead(repo string) (string, error) {
	out, err := gitOut(repo, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve HEAD: %w", err)
	}
	head := strings.TrimSpace(out)
	if head == "" {
		return "", fmt.Errorf("repository has no commits")
	}
	return head, nil
}
