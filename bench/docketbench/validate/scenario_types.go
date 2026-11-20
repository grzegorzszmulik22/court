package main

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// The scenario file formats of spec/04-benchmark.md §2–§4, decoded with
// yaml.v3 strict field checking.

// ---------------------------------------------------------------------------
// commission.yaml
// ---------------------------------------------------------------------------

type doneCheck struct {
	ID  string `yaml:"id"`
	Cmd string `yaml:"cmd"`
}

type budgets struct {
	Tokens      int `yaml:"tokens"`
	Minutes     int `yaml:"minutes"`
	Speculation int `yaml:"speculation"`
}

type commissionFile struct {
	Goal       string      `yaml:"goal"`
	DoneChecks []doneCheck `yaml:"done_checks"`
	Territory  []string    `yaml:"territory"`
	Budgets    budgets     `yaml:"budgets"`
	Template   string      `yaml:"template"`
	Hosts      []string    `yaml:"allowed_hosts"`
}

// ---------------------------------------------------------------------------
// writ (inside script.yaml agents and spawn steps)
// ---------------------------------------------------------------------------

type writSpec struct {
	Categories []string `yaml:"categories"`
	Territory  []string `yaml:"territory"`
	Budgets    budgets  `yaml:"budgets"`
}

// ---------------------------------------------------------------------------
// script.yaml — spec/04 §3's vocabulary
// ---------------------------------------------------------------------------

type scriptFile struct {
	Agents []scriptAgent `yaml:"agents"`
}

type scriptAgent struct {
	ID     string       `yaml:"id"`
	Parent string       `yaml:"parent"`
	Writ   *writSpec    `yaml:"writ"`
	Steps  []scriptStep `yaml:"steps"`
}

// scriptStep is one step: a single-key mapping whose key names the kind.
type scriptStep struct {
	Kind      string
	Backbrief *backbriefStep `yaml:"-"`
	Spawn     *spawnStep     `yaml:"-"`
	Edit      *editStep      `yaml:"-"`
	Tool      *toolStep      `yaml:"-"`
	Declare   *declareStep   `yaml:"-"`
	Commit    *commitStep    `yaml:"-"`
	ClaimDone *claimStep     `yaml:"-"`
	Alarm     *alarmStep     `yaml:"-"`
	Sleep     *sleepStep     `yaml:"-"`
}

var stepKinds = map[string]bool{
	"backbrief": true, "spawn": true, "edit": true, "tool": true,
	"declare": true, "no_decisions": true, "commit": true,
	"claim_done": true, "alarm": true, "sleep": true,
}

func (s *scriptStep) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode || len(n.Content) != 2 {
		return fmt.Errorf("step must be a single-key mapping")
	}
	kind := n.Content[0].Value
	if !stepKinds[kind] {
		return fmt.Errorf("unknown step kind %q", kind)
	}
	s.Kind = kind
	val := n.Content[1]
	var err error
	switch kind {
	case "backbrief":
		err = val.Decode(&s.Backbrief)
	case "spawn":
		err = val.Decode(&s.Spawn)
	case "edit":
		err = val.Decode(&s.Edit)
		if err == nil && s.Edit != nil && s.Edit.Patch == "" && s.Edit.Write == "" && !s.Edit.Delete {
			err = fmt.Errorf("edit step needs patch, write or delete")
		}
	case "tool":
		t := &toolStep{Fields: map[string]any{}}
		if val.Kind != yaml.MappingNode {
			err = fmt.Errorf("tool step wants a mapping")
		} else {
			for i := 0; i+1 < len(val.Content); i += 2 {
				k, v := val.Content[i].Value, val.Content[i+1]
				if k == "name" {
					if derr := v.Decode(&t.Name); derr != nil {
						err = derr
						break
					}
					continue
				}
				var x any
				if derr := v.Decode(&x); derr != nil {
					err = derr
					break
				}
				t.Fields[k] = x
			}
			if err == nil && t.Name == "" {
				err = fmt.Errorf("tool step needs a name")
			}
		}
		s.Tool = t
	case "declare":
		err = val.Decode(&s.Declare)
	case "no_decisions":
		if val.Kind == yaml.MappingNode && len(val.Content) > 0 {
			err = fmt.Errorf("no_decisions takes no fields")
		}
	case "commit":
		err = val.Decode(&s.Commit)
	case "claim_done":
		err = val.Decode(&s.ClaimDone)
	case "alarm":
		err = val.Decode(&s.Alarm)
	case "sleep":
		err = val.Decode(&s.Sleep)
	}
	if err != nil {
		return fmt.Errorf("bad %s step: %w", kind, err)
	}
	return nil
}

type backbriefStep struct {
	PlanFiles []string       `yaml:"plan_files"`
	Expected  []expectedItem `yaml:"expected"`
}

type expectedItem struct {
	Question string   `yaml:"question"`
	Category string   `yaml:"category"`
	Paths    []string `yaml:"paths"`
}

type spawnStep struct {
	ID   string    `yaml:"id"`
	Writ *writSpec `yaml:"writ"`
}

type editStep struct {
	Path   string `yaml:"path"`
	Patch  string `yaml:"patch"`
	Write  string `yaml:"write"`
	Delete bool   `yaml:"delete"`
}

// toolStep is a flat map: name plus arbitrary tool input fields
// (`command`, `url`, `path`, `fail`, ...), per spec/04 §3.
type toolStep struct {
	Name   string
	Fields map[string]any
}

func (t *toolStep) command() string {
	if t == nil {
		return ""
	}
	s, _ := t.Fields["command"].(string)
	return s
}

type declareStep struct {
	Question string   `yaml:"question"`
	Category string   `yaml:"category"`
	Paths    []string `yaml:"paths"`
	Chosen   string   `yaml:"chosen"`
	Petition bool     `yaml:"petition"`
}

type commitStep struct {
	Message string `yaml:"message"`
}

type claimStep struct {
	Task   string   `yaml:"task"`
	Checks []string `yaml:"checks"`
}

type alarmStep struct {
	Text  string   `yaml:"text"`
	Paths []string `yaml:"paths"`
}

type sleepStep struct {
	Ms int `yaml:"ms"`
}

// ---------------------------------------------------------------------------
// registry.yaml — a top-level list of planted entries (spec/04 §4)
// ---------------------------------------------------------------------------

type registryFile struct {
	Entries []registryEntry
}

func (r *registryFile) UnmarshalYAML(n *yaml.Node) error {
	// accept both a bare list (spec form) and {entries: [...]}
	if n.Kind == yaml.SequenceNode {
		return n.Decode(&r.Entries)
	}
	var aux struct {
		Entries []registryEntry `yaml:"entries"`
	}
	if err := n.Decode(&aux); err != nil {
		return err
	}
	r.Entries = aux.Entries
	return nil
}

type registryEntry struct {
	ID          string         `yaml:"id"`
	Type        string         `yaml:"type"`
	Variant     string         `yaml:"variant"`
	Category    string         `yaml:"category"`
	Mast        string         `yaml:"mast"`
	Description string         `yaml:"description"`
	Keywords    []string       `yaml:"keywords"`
	Evidence    []evidenceItem `yaml:"evidence"`
	Source      string         `yaml:"source"`
	Resolution  string         `yaml:"resolution"`
}

// evidenceItem locates the planted item (spec/04 §4):
// {world, commit, path, lines} for file-planted entries; {expected_index,
// paths} for unanswered; {agent, checks, reason} for false-done; two
// {world, path, lines} items for a contest.
type evidenceItem struct {
	World         string   `yaml:"world"`
	Agent         string   `yaml:"agent"`
	Commit        int      `yaml:"commit"`
	Path          string   `yaml:"path"`
	Lines         []int    `yaml:"lines"`
	ExpectedIndex int      `yaml:"expected_index"`
	Paths         []string `yaml:"paths"`
	Checks        []string `yaml:"checks"`
	Reason        string   `yaml:"reason"`
}

// ---------------------------------------------------------------------------
// loader
// ---------------------------------------------------------------------------

func loadYAML(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
