package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Derived state (spec/02 section 6): every table below is a pure function of
// the ledger events and can be dropped and rebuilt by replay. The projector
// updates them inside the append transaction so a crash can never leave the
// chain and the derived state at different points in time.

const derivedDDL = `
CREATE TABLE IF NOT EXISTS commissions(
  commission TEXT PRIMARY KEY, goal TEXT NOT NULL, done_checks TEXT NOT NULL,
  territory TEXT NOT NULL, budgets TEXT NOT NULL, attention INTEGER NOT NULL,
  defaults TEXT NOT NULL, exemptions TEXT NOT NULL,
  opened_at TEXT NOT NULL, closed_at TEXT, outcome TEXT, counts TEXT
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS agents(
  agent TEXT PRIMARY KEY, commission TEXT NOT NULL, parent TEXT NOT NULL,
  tool TEXT NOT NULL, version TEXT NOT NULL, world TEXT NOT NULL,
  writ TEXT NOT NULL, started_at TEXT NOT NULL, stopped_at TEXT, reason TEXT
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS worlds(
  world TEXT PRIMARY KEY, commission TEXT NOT NULL, base TEXT NOT NULL,
  branch TEXT NOT NULL, created_at TEXT NOT NULL, discarded_at TEXT
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS commits(
  commit_id TEXT PRIMARY KEY, commission TEXT NOT NULL, world TEXT NOT NULL,
  parents TEXT NOT NULL, changes TEXT NOT NULL, gated INTEGER NOT NULL,
  assumes TEXT NOT NULL, at TEXT NOT NULL, seq INTEGER NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS decisions(
  commission TEXT NOT NULL, id TEXT NOT NULL, kind TEXT NOT NULL,
  question TEXT NOT NULL, category TEXT NOT NULL, paths TEXT NOT NULL,
  options TEXT NOT NULL, chosen TEXT NOT NULL, petition INTEGER NOT NULL,
  detector TEXT NOT NULL, commit_id TEXT NOT NULL, evidence TEXT NOT NULL,
  intended_default TEXT NOT NULL, agent TEXT NOT NULL, world TEXT NOT NULL,
  sources TEXT NOT NULL, recon TEXT NOT NULL, flags TEXT NOT NULL,
  state TEXT NOT NULL, queued INTEGER NOT NULL, ruling TEXT NOT NULL,
  phrased TEXT NOT NULL, alternative TEXT NOT NULL,
  contested_with TEXT NOT NULL, created_seq INTEGER NOT NULL,
  PRIMARY KEY(commission, id)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS backbriefs(
  commission TEXT NOT NULL, agent TEXT NOT NULL, revision INTEGER NOT NULL,
  goal TEXT NOT NULL, plan TEXT NOT NULL, will_not_touch TEXT NOT NULL,
  at TEXT NOT NULL, seq INTEGER NOT NULL,
  PRIMARY KEY(commission, agent, revision)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS writs(
  id TEXT PRIMARY KEY, commission TEXT NOT NULL, agent TEXT NOT NULL,
  parent TEXT NOT NULL, categories TEXT NOT NULL, class TEXT NOT NULL,
  territory TEXT NOT NULL, budgets TEXT NOT NULL, at TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS writ_refusals(
  seq INTEGER PRIMARY KEY, commission TEXT NOT NULL, agent TEXT NOT NULL,
  parent TEXT NOT NULL, categories TEXT NOT NULL, class TEXT NOT NULL,
  territory TEXT NOT NULL, budgets TEXT NOT NULL, rule TEXT NOT NULL,
  detail TEXT NOT NULL, at TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS claims(
  seq INTEGER PRIMARY KEY, commission TEXT NOT NULL, agent TEXT NOT NULL,
  task TEXT NOT NULL, checks TEXT NOT NULL, at TEXT NOT NULL,
  results TEXT, unverified_by_agent INTEGER
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS alarms(
  seq INTEGER PRIMARY KEY, commission TEXT NOT NULL, agent TEXT NOT NULL,
  text TEXT NOT NULL, paths TEXT NOT NULL, at TEXT NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS gates(
  commission TEXT NOT NULL, agent TEXT NOT NULL, kind TEXT NOT NULL,
  seq INTEGER NOT NULL, PRIMARY KEY(commission, agent)
) WITHOUT ROWID;
`

// derivedTables lists the derived tables in dump order with their row order.
var derivedTables = []struct{ name, order string }{
	{"commissions", "commission"},
	{"agents", "agent"},
	{"worlds", "world"},
	{"commits", "commit_id"},
	{"decisions", "commission, id"},
	{"backbriefs", "commission, agent, revision"},
	{"writs", "id"},
	{"writ_refusals", "seq"},
	{"claims", "seq"},
	{"alarms", "seq"},
	{"gates", "commission, agent"},
}

// execer is the small query interface both *sql.Conn and *sql.Tx satisfy.
type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func j(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func str(v *Value, key string) string {
	x, _ := objGet(v, key)
	return strOf(x)
}

func strSlice(v *Value, key string) []string {
	x, _ := objGet(v, key)
	var out []string
	if x != nil && x.Kind == Array {
		for _, e := range x.Arr {
			out = append(out, strOf(e))
		}
	}
	return out
}

func num(v *Value, key string) int64 {
	x, _ := objGet(v, key)
	f, _ := numOf(x)
	return int64(f)
}

func boolean(v *Value, key string) bool {
	x, _ := objGet(v, key)
	return boolOf(x)
}

func agentID(actor string) string { return strings.TrimPrefix(actor, "agent:") }

func (s *Store) project(ctx context.Context, ev *Event) error {
	return project(s.conn, ctx, ev)
}

// project applies one event to the derived tables (deterministic function of
// the event stream; spec/01 section 11.4).
func project(ex execer, ctx context.Context, ev *Event) error {
	b := ev.Body
	fault("project", ev.Seq)
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		return fmt.Errorf("project %s seq %d: %w", ev.Type, ev.Seq, err)
	}
	exec := func(q string, args ...any) error {
		fault("project-"+tableOf(q), ev.Seq)
		_, err := ex.ExecContext(ctx, q, args...)
		return err
	}
	switch ev.Type {
	case "commission.opened":
		return fail(exec(`INSERT INTO commissions(commission,goal,done_checks,territory,budgets,attention,defaults,exemptions,opened_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			ev.Commission, str(b, "goal"), j(strSlice(b, "done_checks")),
			j(strSlice(b, "territory")), canonObj(b, "budgets"), num(b, "attention"),
			j(strSlice(b, "defaults")), j(strSlice(b, "exemptions")), ev.At))
	case "commission.closed":
		if err := fail(exec(`UPDATE commissions SET closed_at=?, outcome=?, counts=? WHERE commission=?`,
			ev.At, str(b, "outcome"), canonObj(b, "counts"), ev.Commission)); err != nil {
			return err
		}
		return fail(exec(`UPDATE decisions SET state='unanswered', recon='unanswered', queued=1
			WHERE commission=? AND kind='expected' AND state='expected'`, ev.Commission))
	case "agent.started":
		return fail(exec(`INSERT INTO agents(agent,commission,parent,tool,version,world,writ,started_at)
			VALUES(?,?,?,?,?,?,?,?)`,
			str(b, "agent"), ev.Commission, str(b, "parent"), str(b, "tool"),
			str(b, "version"), str(b, "world"), str(b, "writ"), ev.At))
	case "agent.stopped":
		return fail(exec(`UPDATE agents SET stopped_at=?, reason=? WHERE agent=? AND commission=?`,
			ev.At, str(b, "reason"), str(b, "agent"), ev.Commission))
	case "tool.event":
		return nil
	case "world.created":
		return fail(exec(`INSERT INTO worlds(world,commission,base,branch,created_at) VALUES(?,?,?,?,?)`,
			str(b, "world"), ev.Commission, str(b, "base"), str(b, "branch"), ev.At))
	case "world.discarded":
		if err := fail(exec(`UPDATE worlds SET discarded_at=? WHERE world=?`,
			ev.At, str(b, "world"))); err != nil {
			return err
		}
		return fail(exec(`UPDATE decisions SET state='moot', queued=0
			WHERE commission=? AND world=? AND state IN ('open','assumed')`,
			ev.Commission, str(b, "world")))
	case "world.commit":
		if err := fail(exec(`INSERT INTO commits(commit_id,commission,world,parents,changes,gated,assumes,at,seq)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			str(b, "commit"), ev.Commission, str(b, "world"), canonArr(b, "parents"),
			canonArr(b, "changes"), boolInt(boolean(b, "gated")), canonArr(b, "assumes"),
			ev.At, ev.Seq)); err != nil {
			return err
		}
		return fail(exec(`UPDATE decisions SET state='assumed'
			WHERE commission=? AND world=? AND state='open' AND kind='declared' AND chosen<>''`,
			ev.Commission, str(b, "world")))
	case "backbrief.submitted":
		agent := str(b, "agent")
		if agent == "" {
			agent = agentID(ev.Actor)
		}
		var rev int64
		if err := ex.QueryRowContext(ctx,
			`SELECT COUNT(*)+1 FROM backbriefs WHERE commission=? AND agent=?`,
			ev.Commission, agent).Scan(&rev); err != nil {
			return fail(err)
		}
		if err := fail(exec(`INSERT INTO backbriefs(commission,agent,revision,goal,plan,will_not_touch,at,seq)
			VALUES(?,?,?,?,?,?,?,?)`,
			ev.Commission, agent, rev, str(b, "goal"), canonObj(b, "plan"),
			canonArr(b, "will_not_touch"), ev.At, ev.Seq)); err != nil {
			return err
		}
		exp, _ := objGet(b, "expected_decisions")
		if exp != nil && exp.Kind == Array {
			for _, e := range exp.Arr {
				id, err := nextDecisionID(ex, ctx, ev.Commission)
				if err != nil {
					return fail(err)
				}
				if err := fail(exec(`INSERT INTO decisions(commission,id,kind,question,category,paths,options,chosen,petition,
					detector,commit_id,evidence,intended_default,agent,world,sources,recon,flags,state,queued,
					ruling,phrased,alternative,contested_with,created_seq)
					VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
					ev.Commission, id, "expected", str(e, "question"), str(e, "category"),
					canonArr(e, "paths"), "[]", "", 0, "", "", "[]",
					str(e, "intended_default"), "", "", `["expected"]`, "", "[]",
					"expected", 0, "", "", "", "", ev.Seq)); err != nil {
					return err
				}
			}
		}
		return nil
	case "writ.issued":
		return fail(exec(`INSERT INTO writs(id,commission,agent,parent,categories,class,territory,budgets,at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			str(b, "id"), ev.Commission, str(b, "agent"), str(b, "parent"),
			canonArr(b, "categories"), str(b, "class"), canonArr(b, "territory"),
			canonObj(b, "budgets"), ev.At))
	case "writ.refused":
		return fail(exec(`INSERT INTO writ_refusals(seq,commission,agent,parent,categories,class,territory,budgets,rule,detail,at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			ev.Seq, ev.Commission, str(b, "agent"), str(b, "parent"),
			canonArr(b, "categories"), str(b, "class"), canonArr(b, "territory"),
			canonObj(b, "budgets"), str(b, "rule"), str(b, "detail"), ev.At))
	case "decision.declared":
		id, err := nextDecisionID(ex, ctx, ev.Commission)
		if err != nil {
			return fail(err)
		}
		ag := agentID(ev.Actor)
		world := worldOfAgent(ex, ctx, ev.Commission, ag)
		queued := 0
		if boolean(b, "petition") {
			queued = 1
		}
		if err := fail(exec(`INSERT INTO decisions(commission,id,kind,question,category,paths,options,chosen,petition,
			detector,commit_id,evidence,intended_default,agent,world,sources,recon,flags,state,queued,
			ruling,phrased,alternative,contested_with,created_seq)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			ev.Commission, id, "declared", str(b, "question"), str(b, "category"),
			canonArr(b, "paths"), canonArr(b, "options"), str(b, "chosen"),
			boolInt(boolean(b, "petition")), "", "", "[]", "", ag, world,
			`["declared"]`, "declared-only", "[]", "open", queued,
			"", "", "", "", ev.Seq)); err != nil {
			return err
		}
		return fail(exec(`INSERT INTO gates(commission,agent,kind,seq) VALUES(?,?,?,?)
			ON CONFLICT(commission,agent) DO UPDATE SET kind=excluded.kind, seq=excluded.seq`,
			ev.Commission, ag, "declared", ev.Seq))
	case "decision.none":
		return fail(exec(`INSERT INTO gates(commission,agent,kind,seq) VALUES(?,?,?,?)
			ON CONFLICT(commission,agent) DO UPDATE SET kind=excluded.kind, seq=excluded.seq`,
			ev.Commission, agentID(ev.Actor), "none", ev.Seq))
	case "decision.detected":
		id, err := nextDecisionID(ex, ctx, ev.Commission)
		if err != nil {
			return fail(err)
		}
		world := worldOfCommit(ex, ctx, str(b, "commit"))
		ag := agentOfWorld(ex, ctx, world)
		return fail(exec(`INSERT INTO decisions(commission,id,kind,question,category,paths,options,chosen,petition,
			detector,commit_id,evidence,intended_default,agent,world,sources,recon,flags,state,queued,
			ruling,phrased,alternative,contested_with,created_seq)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			ev.Commission, id, "detected", "", str(b, "category"),
			canonArr(b, "paths"), "[]", "", 0, str(b, "detector"), str(b, "commit"),
			canonArr(b, "evidence"), "", ag, world,
			`["detected"]`, "undeclared", "[]", "open", 1,
			"", "", "", "", ev.Seq))
	case "decision.phrased":
		return fail(exec(`UPDATE decisions SET phrased=?, alternative=? WHERE commission=? AND id=?`,
			str(b, "question"), str(b, "alternative"), ev.Commission, str(b, "decision")))
	case "decision.reconciled":
		queued := queuedFor(str(b, "recon"), strSlice(b, "flags"), false, false)
		return fail(exec(`UPDATE decisions SET sources=?, recon=?, flags=?, queued=? WHERE commission=? AND id=?`,
			canonArr(b, "sources"), str(b, "recon"), canonArr(b, "flags"), queued,
			ev.Commission, str(b, "decision")))
	case "decision.contested":
		ds := strSlice(b, "decisions")
		if len(ds) == 2 {
			if err := fail(exec(`UPDATE decisions SET contested_with=?, queued=1 WHERE commission=? AND id=?`,
				ds[1], ev.Commission, ds[0])); err != nil {
				return err
			}
			return fail(exec(`UPDATE decisions SET contested_with=?, queued=1 WHERE commission=? AND id=?`,
				ds[0], ev.Commission, ds[1]))
		}
		return nil
	case "decision.ruled":
		ruling := j(map[string]any{"option": str(b, "option"), "by": "human", "at": ev.At})
		return fail(exec(`UPDATE decisions SET state='ruled', ruling=?, queued=0 WHERE commission=? AND id=?`,
			ruling, ev.Commission, str(b, "decision")))
	case "claim.done":
		return fail(exec(`INSERT INTO claims(seq,commission,agent,task,checks,at) VALUES(?,?,?,?,?,?)`,
			ev.Seq, ev.Commission, agentID(ev.Actor), str(b, "task"),
			canonArr(b, "checks"), ev.At))
	case "claim.checked":
		return fail(exec(`UPDATE claims SET results=?, unverified_by_agent=? WHERE commission=? AND task=?`,
			canonArr(b, "results"), boolInt(boolean(b, "unverified_by_agent")),
			ev.Commission, str(b, "task")))
	case "alarm.raised":
		return fail(exec(`INSERT INTO alarms(seq,commission,agent,text,paths,at) VALUES(?,?,?,?,?,?)`,
			ev.Seq, ev.Commission, agentID(ev.Actor), str(b, "text"),
			canonArr(b, "paths"), ev.At))
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// tableOf extracts the target table name from an INSERT INTO or UPDATE
// statement for fault-point naming.
func tableOf(q string) string {
	w := strings.Fields(strings.TrimSpace(q))
	if len(w) >= 3 && strings.EqualFold(w[0], "INSERT") {
		return strings.TrimRight(w[2], "(")
	}
	if len(w) >= 2 && strings.EqualFold(w[0], "UPDATE") {
		return w[1]
	}
	return "?"
}

func canonObj(v *Value, key string) string {
	x, _ := objGet(v, key)
	if x == nil {
		return ""
	}
	return string(MustCanonical(x))
}

func canonArr(v *Value, key string) string { return canonObj(v, key) }

// nextDecisionID assigns D<n> ids per commission in first-seen order.
func nextDecisionID(ex execer, ctx context.Context, commission string) (string, error) {
	var n int64
	err := ex.QueryRowContext(ctx,
		`SELECT COUNT(*)+1 FROM decisions WHERE commission=?`, commission).Scan(&n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("D%d", n), nil
}

func worldOfAgent(ex execer, ctx context.Context, commission, agent string) string {
	var w string
	err := ex.QueryRowContext(ctx,
		`SELECT world FROM agents WHERE commission=? AND agent=?`, commission, agent).Scan(&w)
	if err != nil {
		return ""
	}
	return w
}

func worldOfCommit(ex execer, ctx context.Context, commit string) string {
	var w string
	err := ex.QueryRowContext(ctx,
		`SELECT world FROM commits WHERE commit_id=?`, commit).Scan(&w)
	if err != nil {
		return ""
	}
	return w
}

func agentOfWorld(ex execer, ctx context.Context, world string) string {
	var a string
	err := ex.QueryRowContext(ctx,
		`SELECT agent FROM agents WHERE world=?`, world).Scan(&a)
	if err != nil {
		return ""
	}
	return a
}

// queuedFor computes the queue rule of spec/01 section 7.3.
func queuedFor(recon string, flags []string, petition, contested bool) int {
	switch recon {
	case "undeclared", "late", "unanswered":
		return 1
	}
	if inSet("outside-writ", flags) || petition || contested {
		return 1
	}
	return 0
}

// recomputedVerdict derives what a decision.reconciled event should carry for
// the decision row, per the version-1 reconciliation rule (single-source
// decisions; the full engine of section 7 arrives in M3.5).
func recomputedVerdict(kind, category, writCategories string, closed bool, petition bool) (sources []string, recon string, flags []string, queued int) {
	sources = []string{kind}
	flags = []string{}
	outside := guaranteeCategories[category]
	if !outside && kind == "declared" && writCategories != "" {
		var cats []string
		if json.Unmarshal([]byte(writCategories), &cats) == nil {
			outside = !inSet(category, cats)
		}
	}
	if outside {
		flags = append(flags, "outside-writ")
	}
	switch kind {
	case "declared":
		recon = "declared-only"
	case "detected":
		recon = "undeclared"
	case "expected":
		if closed {
			recon = "unanswered"
		} else {
			recon = "open"
		}
	}
	queued = queuedFor(recon, flags, petition, false)
	return sources, recon, flags, queued
}
