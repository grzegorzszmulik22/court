package writ

// C2.4.1 — MM2.1: generated attenuation violations, plus the frozen glob
// vectors in test/vectors/writ/.

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"court/internal/ledger"
	"court/internal/paths"
)

func parentWrit() *ledger.WritIssued {
	return &ledger.WritIssued{
		ID: "wroot", Agent: "orch", Parent: "human",
		Categories: []string{"dependency", "schema", "interface", "outbound",
			"territory", "scope", "behaviour", "structure", "other"},
		Class:     "reversible",
		Territory: []string{"src/**", "doc/*"},
		Budgets:   ledger.Budgets{Tokens: 100, Minutes: 50, Speculation: 10},
	}
}

func child() *ledger.WritIssued {
	return &ledger.WritIssued{
		Agent: "c1", Parent: "orch",
		Categories: []string{"interface"},
		Class:      "reversible", Territory: []string{"src/api/**"},
		Budgets: ledger.Budgets{Tokens: 40, Minutes: 20, Speculation: 4},
	}
}

// TestC2_4_1: 5,000 generated writs that break attenuation are all
// refused; valid children pass. Plus the frozen containment vectors.
func TestC2_4_1(t *testing.T) {
	parent := parentWrit()
	rnd := rand.New(rand.NewSource(20261116))
	refused, accepted := 0, 0
	for i := 0; i < 5000; i++ {
		c := child()
		switch rnd.Intn(8) {
		case 0: // extra category the parent never held
			c.Categories = append(c.Categories, "authority")
		case 1: // guarantee category outright
			c.Categories = []string{"guardrail"}
		case 2: // territory outside the parent globs
			c.Territory = []string{"etc/**"}
		case 3: // string-prefix territory that glob semantics rejects
			c.Territory = []string{"src2/x.go"}
		case 4: // budget over the parent's remaining
			c.Budgets.Tokens = 61
		case 5: // a second budget dimension over
			c.Budgets.Minutes = 31
		case 6: // class that is not reversible
			c.Class = "irreversible"
		case 7: // inside the parent's literal run, under a single '*'
			c.Territory = []string{"doc/api/x.md"}
		}
		if rule, _ := Attenuate(parent, c, ledger.Budgets{Tokens: 40, Minutes: 20, Speculation: 4}); rule == "" {
			t.Fatalf("violating writ accepted: %+v", c)
		} else {
			refused++
		}
		// A valid child: narrow one dimension a little more.
		v := child()
		if rule, _ := Attenuate(parent, v, ledger.Budgets{Tokens: 40, Minutes: 20, Speculation: 4}); rule != "" {
			t.Fatalf("valid child refused (%s): %+v", rule, v)
		}
		accepted++
	}
	if refused != 5000 || accepted != 5000 {
		t.Fatalf("MM2.1: refused=%d accepted=%d", refused, accepted)
	}
}

// TestWritVectors: every frozen glob-containment case.
func TestWritVectors(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "test", "vectors", "writ", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no writ vectors: %v", err)
	}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		var v struct {
			Parent, Child string
			Contains      bool
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if got := paths.Contains(v.Parent, v.Child); got != v.Contains {
			t.Errorf("%s: Contains(%q,%q)=%v want %v", f, v.Parent, v.Child, got, v.Contains)
		}
	}
}
