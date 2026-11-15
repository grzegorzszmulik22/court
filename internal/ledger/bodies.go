package ledger

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
)

// Event types of spec/01 section 11.2, in table order.
var typeNames = []string{
	"commission.opened",
	"commission.closed",
	"agent.started",
	"agent.stopped",
	"tool.event",
	"world.created",
	"world.discarded",
	"world.commit",
	"backbrief.submitted",
	"writ.issued",
	"writ.refused",
	"decision.declared",
	"decision.none",
	"decision.detected",
	"decision.phrased",
	"decision.reconciled",
	"decision.contested",
	"decision.ruled",
	"claim.done",
	"claim.checked",
	"alarm.raised",
}

// Categories of spec/01 section 4 (closed list).
var categories = []string{
	"dependency", "schema", "interface", "test-strength", "type-strength",
	"guardrail", "failure-handling", "outbound", "authority", "deletion",
	"territory", "scope", "behaviour", "structure", "other",
}

// Guarantee categories (spec/01 section 4): outside every writ in version 1.
var guaranteeCategories = map[string]bool{
	"test-strength": true, "type-strength": true, "guardrail": true,
	"failure-handling": true, "authority": true, "deletion": true,
}

var reconVerdicts = []string{
	"confirmed", "declared-only", "undeclared", "late", "unanswered", "unforeseen",
}

var reconSources = []string{"declared", "detected", "expected"}

var reconFlags = []string{"late", "unforeseen", "outside-writ"}

func inSet(s string, set []string) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

var reDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ---------------------------------------------------------------------------
// Body types. Field names are the JSON names of api/schema/<type>.schema.json;
// C1.2.5 asserts the bijection.
// ---------------------------------------------------------------------------

// Budgets is the writ/commission budget triple of spec/01 section 9.
type Budgets struct {
	Tokens      int64 `json:"tokens"`
	Minutes     int64 `json:"minutes"`
	Speculation int64 `json:"speculation"`
}

func (b *Budgets) validate() error {
	if b.Tokens < 0 || b.Minutes < 0 || b.Speculation < 0 {
		return fmt.Errorf("budgets must be non-negative")
	}
	return nil
}

// CommissionOpened is the body of commission.opened.
type CommissionOpened struct {
	Goal       string   `json:"goal"`
	DoneChecks []string `json:"done_checks"`
	Territory  []string `json:"territory"`
	Budgets    Budgets  `json:"budgets"`
	Attention  int64    `json:"attention"`
	Defaults   []string `json:"defaults"`
	Exemptions []string `json:"exemptions"`
}

// CommissionClosed is the body of commission.closed.
type CommissionClosed struct {
	Outcome string      `json:"outcome"`
	Counts  ReconCounts `json:"counts"`
}

// ReconCounts tallies decisions per recon verdict (spec/01 section 7.2).
type ReconCounts struct {
	Confirmed    int64 `json:"confirmed"`
	DeclaredOnly int64 `json:"declared-only"`
	Undeclared   int64 `json:"undeclared"`
	Late         int64 `json:"late"`
	Unanswered   int64 `json:"unanswered"`
	Unforeseen   int64 `json:"unforeseen"`
}

// AgentStarted is the body of agent.started.
type AgentStarted struct {
	Agent   string `json:"agent"`
	Parent  string `json:"parent"`
	Tool    string `json:"tool"`
	Version string `json:"version"`
	World   string `json:"world"`
	Writ    string `json:"writ"`
}

// AgentStopped is the body of agent.stopped.
type AgentStopped struct {
	Agent  string `json:"agent"`
	Reason string `json:"reason"`
}

// ToolFields holds the selected input fields kept by spec/01 section 11.3:
// file paths, the command line (first 500 characters), and host names.
type ToolFields struct {
	Paths   []string `json:"paths"`
	Command string   `json:"command"`
	Hosts   []string `json:"hosts"`
}

// ToolEvent is the body of tool.event.
type ToolEvent struct {
	Tool        string     `json:"tool"`
	ToolUseID   string     `json:"tool_use_id"`
	InputDigest string     `json:"input_digest"`
	Fields      ToolFields `json:"fields"`
	Outcome     string     `json:"outcome"`
	Reason      string     `json:"reason,omitempty"`
}

// WorldCreated is the body of world.created.
type WorldCreated struct {
	World  string `json:"world"`
	Base   string `json:"base"`
	Branch string `json:"branch"`
}

// WorldDiscarded is the body of world.discarded.
type WorldDiscarded struct {
	World  string `json:"world"`
	Base   string `json:"base"`
	Branch string `json:"branch"`
}

// PathChange is one changed path in a world.commit.
type PathChange struct {
	Path    string `json:"path"`
	Added   int64  `json:"added"`
	Deleted int64  `json:"deleted"`
}

// WorldCommit is the body of world.commit.
type WorldCommit struct {
	World   string       `json:"world"`
	Commit  string       `json:"commit"`
	Parents []string     `json:"parents"`
	Changes []PathChange `json:"changes"`
	Gated   bool         `json:"gated"`
	Assumes []string     `json:"assumes"`
}

// Plan is the backbrief plan: files it intends to touch and the estimate.
type Plan struct {
	Files []string `json:"files"`
	Lines int64    `json:"lines"`
}

// Expected is one expected decision in a backbrief.
type Expected struct {
	Question        string   `json:"question"`
	Category        string   `json:"category"`
	Paths           []string `json:"paths"`
	IntendedDefault string   `json:"intended_default"`
}

// BackbriefSubmitted is the body of backbrief.submitted.
type BackbriefSubmitted struct {
	Goal              string     `json:"goal"`
	Plan              Plan       `json:"plan"`
	ExpectedDecisions []Expected `json:"expected_decisions"`
	WillNotTouch      []string   `json:"will_not_touch"`
}

// WritIssued is the body of writ.issued.
type WritIssued struct {
	ID         string   `json:"id"`
	Agent      string   `json:"agent"`
	Parent     string   `json:"parent"`
	Categories []string `json:"categories"`
	Class      string   `json:"class"`
	Territory  []string `json:"territory"`
	Budgets    Budgets  `json:"budgets"`
}

// WritRefused is the body of writ.refused: the attempted writ and the
// attenuation rule it violated.
type WritRefused struct {
	Agent      string   `json:"agent"`
	Parent     string   `json:"parent"`
	Categories []string `json:"categories"`
	Class      string   `json:"class"`
	Territory  []string `json:"territory"`
	Budgets    Budgets  `json:"budgets"`
	Rule       string   `json:"rule"`
	Detail     string   `json:"detail"`
}

// Option is one option of a declared decision (spec/01 section 3).
type Option struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	WorldRef string `json:"world_ref,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// DecisionDeclared is the body of decision.declared.
type DecisionDeclared struct {
	Question string   `json:"question"`
	Category string   `json:"category"`
	Paths    []string `json:"paths"`
	Options  []Option `json:"options"`
	Chosen   string   `json:"chosen"`
	Petition bool     `json:"petition"`
}

// DecisionNone is the empty body of decision.none.
type DecisionNone struct {
}

// Evidence is one matched evidence span of decision.detected.
type Evidence struct {
	File      string `json:"file"`
	StartLine int64  `json:"start_line"`
	EndLine   int64  `json:"end_line"`
	Digest    string `json:"digest"`
}

// DecisionDetected is the body of decision.detected.
type DecisionDetected struct {
	Detector string     `json:"detector"`
	Category string     `json:"category"`
	Paths    []string   `json:"paths"`
	Commit   string     `json:"commit"`
	Evidence []Evidence `json:"evidence"`
}

// DecisionPhrased is the body of decision.phrased.
type DecisionPhrased struct {
	Decision    string `json:"decision"`
	Question    string `json:"question"`
	Alternative string `json:"alternative"`
}

// DecisionReconciled is the body of decision.reconciled.
type DecisionReconciled struct {
	Decision string   `json:"decision"`
	Sources  []string `json:"sources"`
	Recon    string   `json:"recon"`
	Flags    []string `json:"flags"`
}

// DecisionContested is the body of decision.contested: two decision ids,
// their worlds, and the differing evidence digests.
type DecisionContested struct {
	Decisions []string `json:"decisions"`
	Worlds    []string `json:"worlds"`
	Evidence  []string `json:"evidence"`
}

// DecisionRuled is the body of decision.ruled.
type DecisionRuled struct {
	Decision string `json:"decision"`
	Option   string `json:"option"`
}

// ClaimDone is the body of claim.done.
type ClaimDone struct {
	Task   string   `json:"task"`
	Checks []string `json:"checks"`
}

// CheckResult is one done-check outcome inside claim.checked.
type CheckResult struct {
	Check  string `json:"check"`
	Result string `json:"result"`
}

// ClaimChecked is the body of claim.checked.
type ClaimChecked struct {
	Task              string        `json:"task"`
	Results           []CheckResult `json:"results"`
	UnverifiedByAgent bool          `json:"unverified_by_agent"`
}

// AlarmRaised is the body of alarm.raised.
type AlarmRaised struct {
	Text  string   `json:"text"`
	Paths []string `json:"paths"`
}

// bodyType maps each event type name to its Go body type. C1.2.5 asserts the
// bijection with api/schema/.
var bodyType = map[string]reflect.Type{
	"commission.opened":   reflect.TypeOf(CommissionOpened{}),
	"commission.closed":   reflect.TypeOf(CommissionClosed{}),
	"agent.started":       reflect.TypeOf(AgentStarted{}),
	"agent.stopped":       reflect.TypeOf(AgentStopped{}),
	"tool.event":          reflect.TypeOf(ToolEvent{}),
	"world.created":       reflect.TypeOf(WorldCreated{}),
	"world.discarded":     reflect.TypeOf(WorldDiscarded{}),
	"world.commit":        reflect.TypeOf(WorldCommit{}),
	"backbrief.submitted": reflect.TypeOf(BackbriefSubmitted{}),
	"writ.issued":         reflect.TypeOf(WritIssued{}),
	"writ.refused":        reflect.TypeOf(WritRefused{}),
	"decision.declared":   reflect.TypeOf(DecisionDeclared{}),
	"decision.none":       reflect.TypeOf(DecisionNone{}),
	"decision.detected":   reflect.TypeOf(DecisionDetected{}),
	"decision.phrased":    reflect.TypeOf(DecisionPhrased{}),
	"decision.reconciled": reflect.TypeOf(DecisionReconciled{}),
	"decision.contested":  reflect.TypeOf(DecisionContested{}),
	"decision.ruled":      reflect.TypeOf(DecisionRuled{}),
	"claim.done":          reflect.TypeOf(ClaimDone{}),
	"claim.checked":       reflect.TypeOf(ClaimChecked{}),
	"alarm.raised":        reflect.TypeOf(AlarmRaised{}),
}

func knownType(t string) bool { _, ok := bodyType[t]; return ok }

// TypeNames returns all event type names of spec/01 section 11.2.
func TypeNames() []string { return append([]string(nil), typeNames...) }

// BodyType returns the Go body type for an event type.
func BodyType(t string) (reflect.Type, bool) { ty, ok := bodyType[t]; return ty, ok }

// ValidateBody checks a decoded body of the given event type against the Go
// rules mirroring api/schema (append path; the ledger verifier uses the JSON
// Schemas themselves).
func ValidateBody(typ string, body any) error {
	badCat := func(c string) error {
		if !inSet(c, categories) {
			return fmt.Errorf("bad category %q", c)
		}
		return nil
	}
	question := func(q string) error {
		if len(q) == 0 || len(q) > 200 || q[len(q)-1] != '?' {
			return fmt.Errorf("question must end with '?' and be <=200 chars")
		}
		return nil
	}
	switch b := body.(type) {
	case *CommissionOpened:
		if b.Goal == "" {
			return fmt.Errorf("goal is empty")
		}
		return b.Budgets.validate()
	case *CommissionClosed:
		if b.Outcome == "" {
			return fmt.Errorf("outcome is empty")
		}
	case *AgentStarted:
		if b.Agent == "" || b.World == "" {
			return fmt.Errorf("agent and world are required")
		}
	case *AgentStopped:
		if b.Agent == "" {
			return fmt.Errorf("agent is required")
		}
	case *ToolEvent:
		if b.Tool == "" {
			return fmt.Errorf("tool is required")
		}
		if !reDigest.MatchString(b.InputDigest) {
			return fmt.Errorf("input_digest is not sha256 hex")
		}
		if len(b.Fields.Command) > 500 {
			return fmt.Errorf("command exceeds 500 characters")
		}
		switch b.Outcome {
		case "ok", "error":
			if b.Reason != "" {
				return fmt.Errorf("reason only allowed when denied")
			}
		case "denied":
			if b.Reason == "" {
				return fmt.Errorf("denied requires a reason")
			}
		default:
			return fmt.Errorf("bad outcome %q", b.Outcome)
		}
	case *WorldCreated:
		if b.World == "" || b.Branch == "" || b.Base == "" {
			return fmt.Errorf("world, base and branch are required")
		}
	case *WorldDiscarded:
		if b.World == "" || b.Branch == "" || b.Base == "" {
			return fmt.Errorf("world, base and branch are required")
		}
	case *WorldCommit:
		if b.World == "" || b.Commit == "" {
			return fmt.Errorf("world and commit are required")
		}
		for _, c := range b.Changes {
			if c.Path == "" {
				return fmt.Errorf("change path is empty")
			}
		}
		for _, a := range b.Assumes {
			if !reDecisID.MatchString(a) {
				return fmt.Errorf("assumes holds bad decision id %q", a)
			}
		}
	case *BackbriefSubmitted:
		if b.Goal == "" {
			return fmt.Errorf("goal is empty")
		}
		for i, e := range b.ExpectedDecisions {
			if err := question(e.Question); err != nil {
				return fmt.Errorf("expected_decisions[%d]: %w", i, err)
			}
			if err := badCat(e.Category); err != nil {
				return err
			}
		}
	case *WritIssued:
		if b.ID == "" || b.Agent == "" {
			return fmt.Errorf("id and agent are required")
		}
		if b.Class != "reversible" {
			return fmt.Errorf("class must be reversible")
		}
		for _, c := range b.Categories {
			if err := badCat(c); err != nil {
				return err
			}
		}
		return b.Budgets.validate()
	case *WritRefused:
		if b.Agent == "" || b.Rule == "" {
			return fmt.Errorf("agent and rule are required")
		}
		if b.Class != "reversible" {
			return fmt.Errorf("class must be reversible")
		}
		for _, c := range b.Categories {
			if err := badCat(c); err != nil {
				return err
			}
		}
		return b.Budgets.validate()
	case *DecisionDeclared:
		if err := question(b.Question); err != nil {
			return err
		}
		if err := badCat(b.Category); err != nil {
			return err
		}
		if len(b.Options) == 0 {
			return fmt.Errorf("options must be non-empty")
		}
		found := false
		for _, o := range b.Options {
			if o.ID == "" {
				return fmt.Errorf("option id is empty")
			}
			if o.ID == b.Chosen {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("chosen %q is not an option", b.Chosen)
		}
	case *DecisionNone:
	case *DecisionDetected:
		if b.Detector == "" {
			return fmt.Errorf("detector is required")
		}
		if err := badCat(b.Category); err != nil {
			return err
		}
		if b.Commit == "" {
			return fmt.Errorf("commit is required")
		}
		for i, e := range b.Evidence {
			if e.File == "" || e.EndLine < e.StartLine || e.StartLine < 1 {
				return fmt.Errorf("evidence[%d] has a bad span", i)
			}
			if !reDigest.MatchString(e.Digest) {
				return fmt.Errorf("evidence[%d] digest is not sha256 hex", i)
			}
		}
	case *DecisionPhrased:
		if !reDecisID.MatchString(b.Decision) {
			return fmt.Errorf("bad decision id")
		}
	case *DecisionReconciled:
		if !reDecisID.MatchString(b.Decision) {
			return fmt.Errorf("bad decision id")
		}
		if len(b.Sources) == 0 || !inSet(b.Recon, reconVerdicts) {
			return fmt.Errorf("bad sources or recon")
		}
		for _, s := range b.Sources {
			if !inSet(s, reconSources) {
				return fmt.Errorf("bad source %q", s)
			}
		}
		for _, f := range b.Flags {
			if !inSet(f, reconFlags) {
				return fmt.Errorf("bad flag %q", f)
			}
		}
	case *DecisionContested:
		if len(b.Decisions) != 2 || len(b.Worlds) != 2 || len(b.Evidence) != 2 {
			return fmt.Errorf("decisions, worlds and evidence each need exactly 2")
		}
		for _, d := range b.Decisions {
			if !reDecisID.MatchString(d) {
				return fmt.Errorf("bad decision id %q", d)
			}
		}
	case *DecisionRuled:
		if !reDecisID.MatchString(b.Decision) {
			return fmt.Errorf("bad decision id")
		}
		if b.Option == "" {
			return fmt.Errorf("option is required")
		}
	case *ClaimDone:
		if b.Task == "" {
			return fmt.Errorf("task is required")
		}
	case *ClaimChecked:
		if b.Task == "" {
			return fmt.Errorf("task is required")
		}
		for _, r := range b.Results {
			if !inSet(r.Result, []string{"pass", "fail", "unrun"}) {
				return fmt.Errorf("bad check result %q", r.Result)
			}
		}
	case *AlarmRaised:
		if b.Text == "" {
			return fmt.Errorf("text is required")
		}
	default:
		return fmt.Errorf("no body type for %q", typ)
	}
	return nil
}

// marshalBody marshals a body struct after normalizing nil slices to empty
// arrays (JSON arrays, never null).
func marshalBody(body any) ([]byte, error) {
	normalizeSlices(reflect.ValueOf(body))
	return json.Marshal(body)
}

// normalizeSlices walks a struct (pointer) and replaces nil slices and maps
// with empty ones so marshaled output keeps the schema's array shape.
func normalizeSlices(v reflect.Value) {
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		normalizeSlices(v.Elem())
		return
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			normalizeSlices(v.Field(i))
		}
	case reflect.Slice:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			return
		}
		for i := 0; i < v.Len(); i++ {
			normalizeSlices(v.Index(i))
		}
	case reflect.Map:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	}
}
