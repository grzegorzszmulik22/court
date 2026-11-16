// Package tools builds the MCP server served at /mcp/<token>: the agent
// tools of spec/02 §5 (record_decision, claim_done, alarm) on the official
// Go SDK, and nothing else — no prompts, resources or sampling.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"court/internal/ledger"
)

// ProtocolVersion is the MCP protocol revision Court pins (the newest the
// pinned SDK v1.8.0 negotiates; C1.6.4 initializes with exactly this).
const ProtocolVersion = "2026-07-28"

// Deps are the per-session hooks the daemon supplies when it builds a
// session's server.
type Deps struct {
	// Actor resolves the ledger actor of the next tool call: the child
	// agent of the parked PreToolUse when the call came through a hook
	// pairing, else the session's own agent.
	Actor func(toolName string) string
	// Emit appends one event under the resolved actor. The call's request
	// context reaches it so the daemon can name the request's own identity
	// (session + JSON-RPC id) for retry deduplication.
	Emit func(ctx context.Context, actor, typ string, body any) error
	// Spawn creates a child world+session under a narrowed writ (M2.4).
	Spawn func(ctx context.Context, actor string, in SpawnIn) (*SpawnOutcome, error)
}

// Categories is the closed list of spec/01 §4 (a copy, so the ledger's
// unexported list stays unexported).
var Categories = []string{
	"dependency", "schema", "interface", "test-strength", "type-strength",
	"guardrail", "failure-handling", "outbound", "authority", "deletion",
	"territory", "scope", "behaviour", "structure", "other",
}

func validCategory(c string) bool {
	for _, x := range Categories {
		if x == c {
			return true
		}
	}
	return false
}

// bad is a tool-result error (isError) — the agent sees the message.
func bad(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}, nil, nil
}

func ok(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

// New builds the session's MCP server: the §5 tools and no other features.
func New(deps Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "court", Version: "1"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name: "record_decision",
		Description: "Declare each decision you make with record_decision before you commit, " +
			"with its question, category, options and chosen option.",
	}, recordDecision(deps))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "claim_done",
		Description: "Claim your task done with claim_done and name the done-checks you ran.",
	}, claimDone(deps))
	mcp.AddTool(s, &mcp.Tool{
		Name: "alarm",
		Description: "Call alarm to bring the human a problem only they can settle; it is " +
			"recorded even if your parent drops it.",
	}, alarm(deps))
	mcp.AddTool(s, &mcp.Tool{
		Name: "backbrief",
		Description: "Submit your backbrief before any other non-read tool: restate the goal, " +
			"give the plan's files and line estimate, and list the decisions you expect.",
	}, backbrief(deps))
	mcp.AddTool(s, &mcp.Tool{
		Name: "spawn",
		Description: "Delegate a narrowed writ to a child agent: categories, territory and " +
			"budgets may only shrink yours; the refusal is recorded and creates nothing.",
	}, spawn(deps))
	return s
}

type optionIn struct {
	ID      string `json:"id" jsonschema:"option id"`
	Summary string `json:"summary" jsonschema:"one-line summary of this option"`
}

type decisionIn struct {
	Question string     `json:"question" jsonschema:"the decision's question"`
	Category string     `json:"category" jsonschema:"decision category"`
	Paths    []string   `json:"paths" jsonschema:"paths the decision touches"`
	Options  []optionIn `json:"options" jsonschema:"the options you weighed"`
	Chosen   string     `json:"chosen" jsonschema:"id of the chosen option"`
	Petition bool       `json:"petition,omitempty" jsonschema:"true when the choice needs the human"`
}

type claimDoneIn struct {
	Task   string   `json:"task" jsonschema:"the task being claimed done"`
	Checks []string `json:"checks" jsonschema:"done-check ids you ran"`
}

type alarmIn struct {
	Text  string   `json:"text" jsonschema:"what the human needs to settle"`
	Paths []string `json:"paths" jsonschema:"paths the alarm is about"`
}

type planIn struct {
	Files []string `json:"files" jsonschema:"files the plan intends to touch"`
	Lines int64    `json:"lines" jsonschema:"estimated changed lines"`
}

type expectedIn struct {
	Question string `json:"question" jsonschema:"the decision's question"`
	// Category is optional in the wire schema so a call that omits it is
	// rejected by the tool with the valid list, not by schema validation
	// (C2.2.2).
	Category        string   `json:"category,omitempty" jsonschema:"decision category"`
	Paths           []string `json:"paths" jsonschema:"paths the decision touches"`
	IntendedDefault string   `json:"intended_default" jsonschema:"the option taken absent a ruling"`
}

type backbriefIn struct {
	Goal              string       `json:"goal" jsonschema:"the goal restated in your own words"`
	Plan              planIn       `json:"plan" jsonschema:"files and line estimate"`
	ExpectedDecisions []expectedIn `json:"expected_decisions" jsonschema:"decisions you expect to face"`
	WillNotTouch      []string     `json:"will_not_touch" jsonschema:"paths outside the plan"`
}

func recordDecision(deps Deps) mcp.ToolHandlerFor[decisionIn, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in decisionIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Question) == "" {
			return bad("question is required")
		}
		if !validCategory(in.Category) {
			return bad("unknown category %q; valid categories: %s",
				in.Category, strings.Join(Categories, ", "))
		}
		if len(in.Options) == 0 {
			return bad("options is required and must not be empty")
		}
		found := false
		for _, o := range in.Options {
			if o.ID == in.Chosen {
				found = true
			}
		}
		if !found {
			return bad("chosen %q is not one of the option ids", in.Chosen)
		}
		opts := make([]ledger.Option, len(in.Options))
		for i, o := range in.Options {
			opts[i] = ledger.Option{ID: o.ID, Summary: o.Summary}
		}
		err := deps.Emit(ctx, deps.Actor("record_decision"), "decision.declared",
			&ledger.DecisionDeclared{
				Question: in.Question, Category: in.Category, Paths: in.Paths,
				Options: opts, Chosen: in.Chosen, Petition: in.Petition})
		if err != nil {
			return bad("not recorded: %v", err)
		}
		return ok("decision recorded")
	}
}

func claimDone(deps Deps) mcp.ToolHandlerFor[claimDoneIn, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in claimDoneIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Task) == "" {
			return bad("task is required")
		}
		err := deps.Emit(ctx, deps.Actor("claim_done"), "claim.done",
			&ledger.ClaimDone{Task: in.Task, Checks: in.Checks})
		if err != nil {
			return bad("not recorded: %v", err)
		}
		return ok("claim recorded")
	}
}

func alarm(deps Deps) mcp.ToolHandlerFor[alarmIn, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in alarmIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Text) == "" {
			return bad("text is required")
		}
		err := deps.Emit(ctx, deps.Actor("alarm"), "alarm.raised",
			&ledger.AlarmRaised{Text: in.Text, Paths: in.Paths})
		if err != nil {
			return bad("not recorded: %v", err)
		}
		return ok("alarm raised")
	}
}

// backbrief records backbrief.submitted; the event is the ordering gate's
// release: nothing but read-only tools may run before it (M2.2). A second
// submission is a revision — append-only keeps both (C2.2.4).
func backbrief(deps Deps) mcp.ToolHandlerFor[backbriefIn, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in backbriefIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Goal) == "" {
			return bad("goal is required")
		}
		exp := make([]ledger.Expected, len(in.ExpectedDecisions))
		for i, e := range in.ExpectedDecisions {
			if !validCategory(e.Category) {
				return bad("expected_decisions[%d]: unknown category %q; valid categories: %s",
					i, e.Category, strings.Join(Categories, ", "))
			}
			exp[i] = ledger.Expected{
				Question: e.Question, Category: e.Category,
				Paths: e.Paths, IntendedDefault: e.IntendedDefault,
			}
		}
		err := deps.Emit(ctx, deps.Actor("backbrief"), "backbrief.submitted",
			&ledger.BackbriefSubmitted{
				Goal:              in.Goal,
				Plan:              ledger.Plan{Files: in.Plan.Files, Lines: in.Plan.Lines},
				ExpectedDecisions: exp, WillNotTouch: in.WillNotTouch})
		if err != nil {
			return bad("not recorded: %v", err)
		}
		return ok("backbrief recorded")
	}
}

type SpawnIn struct {
	Agent      string   `json:"agent" jsonschema:"the child agent's id"`
	Task       string   `json:"task" jsonschema:"the child's task"`
	Categories []string `json:"categories" jsonschema:"categories the child may decide alone"`
	Territory  []string `json:"territory" jsonschema:"path globs the child may touch"`
	Budgets    struct {
		Tokens      int64 `json:"tokens"`
		Minutes     int64 `json:"minutes"`
		Speculation int64 `json:"speculation"`
	} `json:"budgets" jsonschema:"budgets carved out of yours"`
}

// SpawnOutcome is the daemon's answer to a successful spawn.
type SpawnOutcome struct {
	Token    string `json:"token"`
	HookURL  string `json:"hook_url"`
	MCPURL   string `json:"mcp_url"`
	World    string `json:"world"`
	Worktree string `json:"worktree"`
	Writ     string `json:"writ"`
	Agent    string `json:"agent"`
}

// spawn delegates a narrowed writ to a child agent (spec/01 §9). The
// daemon attenuates against the caller's writ; a refusal records
// writ.refused and creates nothing.
func spawn(deps Deps) mcp.ToolHandlerFor[SpawnIn, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SpawnIn) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Agent) == "" {
			return bad("agent is required")
		}
		if deps.Spawn == nil {
			return bad("spawn is not available here")
		}
		res, err := deps.Spawn(ctx, deps.Actor("spawn"), in)
		if err != nil {
			return bad("%v", err)
		}
		raw, _ := json.Marshal(res)
		return ok("spawned: " + string(raw))
	}
}
