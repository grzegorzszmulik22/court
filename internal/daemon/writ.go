package daemon

// Writs and spawn (spec/01 §9, milestone M2.4): attenuation, the root
// writ, child world+session creation, and the outside-writ mark on
// declared decisions.

import (
	"encoding/json"
	"fmt"
	"net/http"

	"court/internal/ledger"
	"court/internal/world"
	cwrit "court/internal/writ"
)

// remarshal maps a decoded body back into a typed body.
func remarshal(b any, into any) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

// IssueWrit records a writ.issued after validation. The human's root
// writ (parent "human") skips attenuation — the human grants directly;
// guarantee categories are refused in any writ.
func (d *Daemon) IssueWrit(commission string, w *ledger.WritIssued) error {
	if w.ID == "" {
		id, err := randomToken()
		if err != nil {
			return err
		}
		w.ID = "w" + id[:12]
	}
	if w.Parent != "human" {
		parent := d.writs[d.writByAgent[commission+"/"+w.Parent]]
		if parent == nil {
			return fmt.Errorf("daemon: agent %s holds no writ", w.Parent)
		}
		if rule, detail := cwrit.Attenuate(parent, w, d.spent[parent.ID]); rule != "" {
			_, _, err := d.emit(commission, "court", "writ.refused",
				&ledger.WritRefused{
					Agent: w.Agent, Parent: w.Parent, Categories: w.Categories,
					Class: w.Class, Territory: w.Territory, Budgets: w.Budgets,
					Rule: rule, Detail: detail},
				"writ.refused|"+commission+"/"+w.Agent)
			if err != nil {
				return err
			}
			return fmt.Errorf("writ refused (%s): %s", rule, detail)
		}
		d.spent[parent.ID] = addB(d.spent[parent.ID], w.Budgets)
	} else if rule, detail := checkClassCats(w); rule != "" {
		return fmt.Errorf("writ refused (%s): %s", rule, detail)
	}
	_, _, err := d.emit(commission, "court", "writ.issued", w,
		"writ|"+w.ID)
	if err != nil {
		return err
	}
	d.writs[w.ID] = w
	d.writByAgent[commission+"/"+w.Agent] = w.ID
	return nil
}

func checkClassCats(w *ledger.WritIssued) (rule, detail string) {
	if w.Class != "reversible" {
		return "class", fmt.Sprintf("class %q is not reversible", w.Class)
	}
	for _, c := range w.Categories {
		if ledger.IsGuarantee(c) {
			return "categories", fmt.Sprintf("guarantee category %q is outside every writ", c)
		}
	}
	return "", ""
}

func addB(a, b ledger.Budgets) ledger.Budgets {
	a.Tokens += b.Tokens
	a.Minutes += b.Minutes
	a.Speculation += b.Speculation
	return a
}

// WritFor resolves the writ a session runs under.
func (d *Daemon) WritFor(sess *Session) *ledger.WritIssued {
	return d.writs[sess.Writ]
}

// withinWrit reports whether a declared decision sits inside the
// session's writ: its category is granted (guarantee categories never
// are) and every named path is covered by the territory.
func (d *Daemon) withinWrit(sess *Session, dd *ledger.DecisionDeclared) bool {
	w := d.WritFor(sess)
	if w == nil {
		return true // no writ recorded: nothing to be outside of
	}
	inside := false
	for _, c := range w.Categories {
		if c == dd.Category {
			inside = true
		}
	}
	if !inside {
		return false
	}
	for _, p := range dd.Paths {
		if !cwrit.Covers(w.Territory, p) {
			return false
		}
	}
	return true
}

// SpawnResult is what a successful spawn hands back to the caller.
type SpawnResult struct {
	Token    string `json:"token"`
	HookURL  string `json:"hook_url"`
	MCPURL   string `json:"mcp_url"`
	World    string `json:"world"`
	Worktree string `json:"worktree"`
	Writ     string `json:"writ"`
	Agent    string `json:"agent"`
}

// SpawnRequest is the spawn tool's payload.
type SpawnRequest struct {
	Agent      string         `json:"agent"`
	Task       string         `json:"task"`
	Categories []string       `json:"categories"`
	Territory  []string       `json:"territory"`
	Budgets    ledger.Budgets `json:"budgets"`
}

// Spawn applies §9 attenuation against the session's writ and, on
// success, forks the child world, registers its session and records
// writ.issued. A refusal records writ.refused and creates nothing.
func (d *Daemon) Spawn(sess *Session, req SpawnRequest) (*SpawnResult, error) {
	child := &ledger.WritIssued{
		Agent: req.Agent, Parent: sess.Agent, Categories: req.Categories,
		Class: "reversible", Territory: req.Territory, Budgets: req.Budgets,
	}
	id, err := randomToken()
	if err != nil {
		return nil, err
	}
	child.ID = "w" + id[:12]
	parent := d.writs[sess.Writ]
	if parent == nil {
		return nil, fmt.Errorf("agent %s holds no writ", sess.Agent)
	}
	if rule, detail := cwrit.Attenuate(parent, child, d.spent[parent.ID]); rule != "" {
		_, _, err := d.emit(sess.Commission, "court", "writ.refused",
			&ledger.WritRefused{
				Agent: req.Agent, Parent: sess.Agent, Categories: req.Categories,
				Class: "reversible", Territory: req.Territory, Budgets: req.Budgets,
				Rule: rule, Detail: detail})
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("spawn refused (%s): %s", rule, detail)
	}
	// Fork the child's world under the commission's worlds root.
	root, err := world.Open(d.opts.Repo, d.opts.DataDir, sess.Commission)
	if err != nil {
		return nil, err
	}
	base := sess.Base
	if base == "" {
		base = "HEAD"
	}
	wname := sess.World + "-" + req.Agent
	gw, err := root.Fork(world.Commit(base), wname)
	if err != nil {
		return nil, err
	}
	ggw, ok := gw.(*world.GitWorld)
	if !ok {
		return nil, fmt.Errorf("unexpected world backend")
	}
	if _, _, err := d.emit(sess.Commission, "court", "writ.issued", child,
		"writ|"+child.ID); err != nil {
		return nil, err
	}
	d.writs[child.ID] = child
	d.writByAgent[sess.Commission+"/"+child.Agent] = child.ID
	d.spent[parent.ID] = addB(d.spent[parent.ID], child.Budgets)
	token, err := d.startSession(Session{
		Commission: sess.Commission, Agent: req.Agent, Parent: sess.Agent,
		Tool: sess.Tool, Version: sess.Version,
		World: wname, Base: base, Branch: ggw.Branch(),
		Worktree: ggw.Path(), Writ: child.ID, Task: req.Task,
		RequireBackbrief: sess.RequireBackbrief,
	})
	if err != nil {
		return nil, err
	}
	return &SpawnResult{
		Token: token, HookURL: d.URL() + "/hook/" + token,
		MCPURL: d.URL() + "/mcp/" + token,
		World:  wname, Worktree: ggw.Path(), Writ: child.ID, Agent: req.Agent,
	}, nil
}

// adminWrit issues a writ via the admin API (the human's root writ).
func (d *Daemon) adminWrit(w http.ResponseWriter, r *http.Request) {
	if !d.adminOK(w, r) {
		return
	}
	var req struct {
		Commission string            `json:"commission"`
		Writ       ledger.WritIssued `json:"writ"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		d.refuse(w, r, "admin", "bad writ body", http.StatusBadRequest)
		return
	}
	if err := d.IssueWrit(req.Commission, &req.Writ); err != nil {
		d.refuse(w, r, "admin", err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, 200, map[string]any{"writ": req.Writ.ID})
}
