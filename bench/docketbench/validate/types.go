package main

// The scenario file formats of spec/04-benchmark.md §2–§4, decoded with
// yaml.v3 strict field checking. These are the authoritative shapes for
// M3.1; bench/docketbench/README.md documents them in prose.

// ---------------------------------------------------------------------------
// categories (spec/01 §4)
// ---------------------------------------------------------------------------

var categories = map[string]bool{
	"dependency": true, "schema": true, "interface": true,
	"test-strength": true, "type-strength": true, "guardrail": true,
	"failure-handling": true, "outbound": true, "authority": true,
	"deletion": true, "territory": true, "scope": true,
	"behaviour": true, "structure": true, "other": true,
}

// guaranteeCategories always get their own ruling; they are outside every
// writ in version 1 (spec/01 §4, §9).
var guaranteeCategories = map[string]bool{
	"test-strength": true, "type-strength": true, "guardrail": true,
	"failure-handling": true, "authority": true, "deletion": true,
}

// detectorCategory is spec/03 §2's detector-to-category table; behaviour,
// structure and other have no detector in version 1.
var detectorCategory = map[string]string{
	"DT1": "dependency", "DT2": "schema", "DT3": "interface",
	"DT4": "test-strength", "DT5": "type-strength", "DT6": "guardrail",
	"DT7": "failure-handling", "DT8": "outbound", "DT9": "authority",
	"DT10": "deletion", "DT11": "territory", "DT12": "scope",
}

// detectorSubkinds is spec/03 §2's subkind list per detector.
var detectorSubkinds = map[string][]string{
	"DT1":  {"added", "removed", "version", "lock-mismatch"},
	"DT2":  {"migration", "ddl", "format"},
	"DT3":  {"removed", "changed", "added"},
	"DT4":  {"deleted_test", "skip_or_focus", "fewer_assertions", "changed_expectation", "loosened_threshold"},
	"DT5":  {"widened", "suppressed"},
	"DT6":  {"ci", "lint", "threshold", "strictness", "hook", "suppressed"},
	"DT7":  {"swallowed", "broadened", "special-case"},
	"DT8":  {"code", "agent"},
	"DT9":  {"touched", "widened"},
	"DT10": {"sql", "file", "command"},
	"DT11": {},
	"DT12": {"unplanned-file", "oversize", "new-top-level"},
}

// vectorLangs is the version-1 language set every detector supports
// (spec/03 §2.1).
var vectorLangs = []string{"go", "python", "typescript"}

// namedNearMisses: the near-miss shapes spec/03 §2 and the M3.1 build text
// require among each detector's negatives. Every detector must show each
// of its named shapes at least once per language.
var namedNearMisses = map[string][]string{
	"DT1": {"dependency-reordered"},
	"DT4": {"assertion-moved", "threshold-tightened"},
	"DT8": {"comment-hostname"},
}

// honestCIKinds are the two DT4 line-pairing checks whose false-positive
// shapes must each have at least 10 negative vectors per language
// (M3 check C3.3.6, sourced from the honest-ci report).
var honestCIKinds = []string{"changed_expectation", "fewer_assertions"}

// ---------------------------------------------------------------------------
// research citations
// ---------------------------------------------------------------------------

// researchSources maps each citation slug allowed in a registry `source`
// field to a string that must literally appear in research/1_detection.md.
// A citation is written "research/1_detection.md#<slug>".
var researchSources = map[string]string{
	"arctic":               "ARCTIC",
	"adrchive":             "adrchive",
	"transluce":            "Transluce",
	"episteme":             "EPISTEME",
	"codeclone":            "CodeClone",
	"claim-plane":          "Claim Plane",
	"gated-workflow":       "Gated Spec-Driven Workflow",
	"overclaimbench":       "OverclaimBench",
	"effectmatch":          "EffectMatch",
	"aporia":               "Aporia",
	"assumptionminer":      "AssumptionMiner",
	"honest-ci":            "honest-ci",
	"repopilot":            "repopilot",
	"swarm-orchestrator":   "swarm-orchestrator",
	"testing-with-agents":  "Testing with Coding Agents",
	"impossiblebench":      "ImpossibleBench",
	"frognano":             "FrogNano",
	"verification-horizon": "Verification Horizon",
	"trace":                "TRACE",
	"rh-detect":            "RH-Detect",
	"hacking-or-failing":   "Hacking or Just Failing",
	"hodoscope":            "Hodoscope",
	"testjack":             "TestJack",
	"specguard":            "SpecGuard",
	"wink":                 "Wink",
	"overeager-bench":      "OverEager-Bench",
	"silent-forks":         "Over-Defense in Coding Agents Begins at Silent Forks",
	"swe-misbehavior":      "SWE-Misbehavior",
	"anchor":               "ANCHOR",
	"swe-chat":             "SWE-chat",
	"library-usage":        "A Study of Library Usage in Agent-Authored Pull Requests",
	"depdec-bench":         "DepDec-Bench",
	"safer-builders":       "Safer Builders, Risky Maintainers",
}
