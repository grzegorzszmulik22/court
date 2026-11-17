package commission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"court/internal/ledger"
)

// The blind clerk (M2.3, spec/02 §7): a tool-less one-shot session given
// only the commission, asked for expected decisions in the backbrief
// schema. Two hermetic drivers exist: --clerk <file> replays a recorded
// output, --clerk-cmd <command> runs a command with the request JSON on
// stdin and takes its stdout as the output. With neither, no model is
// configured and the commission proceeds on the orchestrator's list.

// clerkRequest is the clerk's entire input: the commission, never the
// orchestrator's backbrief and never repository contents (C2.3.1). The
// commissioner writes it under the agent's state dir — it is the
// recorded request the check asserts on.
type clerkRequest struct {
	Commission string `json:"commission"`
	Goal       string `json:"goal"`
	Schema     string `json:"schema"`
}

const clerkSchema = `{"expected_decisions":[{"question":"...?","category":"<category>","paths":["..."],"intended_default":"..."}]}`

// WriteClerkRequest records the exact request the clerk driver sees and
// returns its file path.
func WriteClerkRequest(dir, commission, goal string) (string, []byte, error) {
	req := clerkRequest{Commission: commission, Goal: goal, Schema: clerkSchema}
	raw, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	p := filepath.Join(dir, "clerk-request.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		return "", nil, err
	}
	return p, raw, nil
}

// RunClerk produces the clerk's raw output: the fixture file's contents
// or the command's stdout. Empty output with a nil error means no model
// is configured.
func RunClerk(o *Options, reqRaw []byte) (string, error) {
	switch {
	case o.Clerk != "":
		raw, err := os.ReadFile(o.Clerk)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	case o.ClerkCmd != "":
		cmd := exec.Command("sh", "-c", o.ClerkCmd)
		cmd.Stdin = bytes.NewReader(reqRaw)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("clerk command: %w: %s", err, errb.String())
		}
		return out.String(), nil
	}
	return "", nil
}

// ParseClerkOutput validates raw against the expected-decision schema:
// an object carrying expected_decisions (or a bare list), every entry
// with a '?'-terminated question of at most 200 characters, a valid
// category, string paths and an intended_default.
func ParseClerkOutput(raw string) ([]ledger.Expected, error) {
	var wire struct {
		Expected []ledger.Expected `json:"expected_decisions"`
	}
	trim := bytes.TrimSpace([]byte(raw))
	if len(trim) == 0 {
		return nil, fmt.Errorf("empty clerk output")
	}
	if trim[0] == '[' {
		if err := json.Unmarshal(trim, &wire.Expected); err != nil {
			return nil, err
		}
	} else if err := json.Unmarshal(trim, &wire); err != nil {
		return nil, err
	}
	body := &ledger.BackbriefSubmitted{
		Goal:              "clerk",
		Plan:              ledger.Plan{},
		ExpectedDecisions: wire.Expected,
		WillNotTouch:      []string{},
	}
	if err := ledger.ValidateBody("backbrief.submitted", body); err != nil {
		return nil, err
	}
	return wire.Expected, nil
}
