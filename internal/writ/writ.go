// Package writ holds the attenuation rule of spec/01 §9: a child's writ
// may only narrow its parent's — categories a subset, every territory
// glob contained in the parent's, every budget within the parent's
// remaining (budgets minus what earlier children took).
package writ

import (
	"fmt"

	"court/internal/ledger"
	"court/internal/paths"
)

// Root is the human's writ to the orchestrator: every non-guarantee
// category, class reversible, the commission's territory and budgets.
func Root(agent string, territory []string, budgets ledger.Budgets) *ledger.WritIssued {
	var cats []string
	for _, c := range ledger.Categories() {
		if !ledger.IsGuarantee(c) {
			cats = append(cats, c)
		}
	}
	return &ledger.WritIssued{
		Agent: agent, Parent: "human", Categories: cats,
		Class: "reversible", Territory: territory, Budgets: budgets,
	}
}

// Attenuate returns the violated rule and detail when req is not a
// narrowing of parent, or empty strings when it issues.
func Attenuate(parent, req *ledger.WritIssued, spent ledger.Budgets) (rule, detail string) {
	if req.Class != "reversible" {
		return "class", fmt.Sprintf("class %q is not reversible", req.Class)
	}
	for _, c := range req.Categories {
		if ledger.IsGuarantee(c) {
			return "categories", fmt.Sprintf("guarantee category %q is outside every writ", c)
		}
	}
	if !subset(req.Categories, parent.Categories) {
		return "categories", fmt.Sprintf("categories %v not within parent's %v", req.Categories, parent.Categories)
	}
	for _, g := range req.Territory {
		if !covered(parent.Territory, g) {
			return "territory", fmt.Sprintf("glob %q not contained in parent's territory %v", g, parent.Territory)
		}
	}
	rem := ledger.Budgets{
		Tokens:      parent.Budgets.Tokens - spent.Tokens,
		Minutes:     parent.Budgets.Minutes - spent.Minutes,
		Speculation: parent.Budgets.Speculation - spent.Speculation,
	}
	if req.Budgets.Tokens > rem.Tokens {
		return "budgets", fmt.Sprintf("tokens %d exceeds parent's remaining %d", req.Budgets.Tokens, rem.Tokens)
	}
	if req.Budgets.Minutes > rem.Minutes {
		return "budgets", fmt.Sprintf("minutes %d exceeds parent's remaining %d", req.Budgets.Minutes, rem.Minutes)
	}
	if req.Budgets.Speculation > rem.Speculation {
		return "budgets", fmt.Sprintf("speculation %d exceeds parent's remaining %d", req.Budgets.Speculation, rem.Speculation)
	}
	return "", ""
}

// Covers reports whether the writ's territory covers a path (for the
// outside-writ mark): the path (literal or glob) is contained in some
// territory glob.
func Covers(territory []string, path string) bool { return covered(territory, path) }

func covered(territory []string, g string) bool {
	for _, p := range territory {
		if paths.Contains(p, g) {
			return true
		}
	}
	return false
}

func subset(child, parent []string) bool {
	for _, c := range child {
		found := false
		for _, p := range parent {
			if c == p {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
