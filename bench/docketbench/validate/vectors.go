package main

// Detection-vector validation (M3.1 build item 1, check C3.1.1):
//   test/vectors/detect/DT<n>/<lang>/{pos|neg}-NN.json
//
// The validator verifies everything about a vector that can be checked
// without implementing the detectors: shape, vocabulary, counts, that a
// hit's evidence text sits on its stated lines inside the changed region
// of the tree diff, and that every required near-miss and false-positive
// shape appears among the negatives.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type dvector struct {
	ID         string                 `json:"id"`
	Detector   string                 `json:"detector"`
	Language   string                 `json:"language"`
	Files      map[string]dvectorFile `json:"files"`
	Events     []dvectorEvent         `json:"tool_events"`
	Commission *dvectorCommission     `json:"commission"`
	Expect     []dvectorHit           `json:"expect"`
	NearMiss   string                 `json:"near_miss"`
	FPKind     string                 `json:"fp_kind"`
	Note       string                 `json:"note"`
}

// dvectorCommission carries the commission fields the commission-aware
// detectors read (DT8 allowed hosts, DT11 territory, DT12 plan).
type dvectorCommission struct {
	Territory    []string `json:"territory"`
	AllowedHosts []string `json:"allowed_hosts"`
	PlanFiles    []string `json:"plan_files"`
	PlanLines    int      `json:"plan_lines"`
}

type dvectorFile struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type dvectorEvent struct {
	Name    string         `json:"name"`
	Input   map[string]any `json:"input"`
	Outcome string         `json:"outcome"`
}

type dvectorHit struct {
	Detector string `json:"detector"`
	Category string `json:"category"`
	Subkind  string `json:"subkind"`
	Path     string `json:"path"`
	Lines    [2]int `json:"lines"`
	Side     string `json:"side"` // after | before | events
	Evidence string `json:"evidence"`
	Rule     string `json:"rule"`
}

var vectorFileRe = regexp.MustCompile(`^(pos|neg)-\d+\.json$`)

func validateVectors(root string, v *violations) {
	dts, err := os.ReadDir(root)
	if err != nil {
		v.add("vectors-missing", root, err.Error())
		return
	}
	seenDT := map[string]bool{}
	for _, d := range dts {
		if !d.IsDir() {
			continue
		}
		dt := d.Name()
		if _, ok := detectorCategory[dt]; !ok {
			v.add("vectors-detector", dt, "directory is not a detector DT1..DT12")
			continue
		}
		seenDT[dt] = true
		checkOneDetector(root, dt, v)
	}
	for dt := range detectorCategory {
		if !seenDT[dt] {
			v.add("vectors-detector", dt, "no vector directory")
		}
	}
}

func checkOneDetector(root, dt string, v *violations) {
	dtdir := filepath.Join(root, dt)
	rules := readRulesDoc(dtdir, dt, v)
	for _, lang := range vectorLangs {
		ldir := filepath.Join(dtdir, lang)
		fi, err := os.Stat(ldir)
		if err != nil || !fi.IsDir() {
			v.add("vectors-lang", dt+"/"+lang, "missing language directory")
			continue
		}
		checkLangDir(ldir, dt, lang, rules, v)
	}
}

func readRulesDoc(dtdir, dt string, v *violations) map[string]bool {
	out := map[string]bool{}
	raw, err := os.ReadFile(filepath.Join(dtdir, "RULES.md"))
	if err != nil {
		v.add("vectors-rules", dt, "RULES.md missing (03 §2.1: line rules documented per detector)")
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- `") {
			rest := strings.TrimPrefix(line, "- ")
			for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(rest, -1) {
				rule := m[1]
				if strings.Contains(rule, "<lang>") {
					for _, l := range vectorLangs {
						out[strings.ReplaceAll(rule, "<lang>", l)] = true
					}
				} else {
					out[rule] = true
				}
			}
		}
	}
	if len(out) == 0 {
		v.add("vectors-rules", dt, "RULES.md lists no rule ids")
	}
	return out
}

func checkLangDir(ldir, dt, lang string, rules map[string]bool, v *violations) {
	where := dt + "/" + lang
	files, err := os.ReadDir(ldir)
	if err != nil {
		v.add("vectors-lang", where, err.Error())
		return
	}
	pos, neg := 0, 0
	nearSeen := map[string]int{}
	fpSeen := map[string]int{}
	rulesUsed := map[string]bool{}
	ids := map[string]bool{}
	sigs := map[string]string{} // normalized case signature -> first vector id
	for _, f := range files {
		m := vectorFileRe.FindStringSubmatch(f.Name())
		if m == nil {
			if strings.HasSuffix(f.Name(), ".json") {
				v.addf("vectors-name", where, "file %s is not {pos|neg}-NN.json", f.Name())
			}
			continue
		}
		positive := m[1] == "pos"
		vec := loadVector(filepath.Join(ldir, f.Name()), where+"/"+f.Name(), v)
		if vec == nil {
			continue
		}
		checkVector(vec, positive, dt, lang, rules, where, v)
		if ids[vec.ID] {
			v.addf("vectors-id", where, "duplicate vector id %q", vec.ID)
		}
		ids[vec.ID] = true
		// F1: vectors are distinct cases — two vectors that differ only in
		// renamed identifiers or digits are the same situation.
		sig := vectorSignature(vec)
		if prev, dup := sigs[sig]; dup {
			v.addf("vectors-duplicate", where+"/"+f.Name(),
				"same case as %s after normalising identifiers and digits", prev)
		} else {
			sigs[sig] = f.Name()
		}
		if positive {
			pos++
			for _, h := range vec.Expect {
				if h.Detector == dt {
					rulesUsed[h.Rule] = true
				}
			}
		} else {
			neg++
			if vec.NearMiss != "" {
				nearSeen[vec.NearMiss]++
			}
			if vec.FPKind != "" {
				fpSeen[vec.FPKind]++
			}
		}
	}
	if pos < 20 {
		v.addf("vectors-count", where, "%d positives < 20", pos)
	}
	if neg < 20 {
		v.addf("vectors-count", where, "%d negatives < 20", neg)
	}
	for _, nm := range namedNearMisses[dt] {
		if nearSeen[nm] == 0 {
			v.addf("vectors-nearmiss", where, "near miss %q never appears among negatives", nm)
		}
	}
	if dt == "DT4" {
		for _, k := range honestCIKinds {
			if fpSeen[k] < 10 {
				v.addf("vectors-fp", where, "fewer than 10 %q false-positive negatives (C3.3.6)", k)
			}
		}
	}
	for r := range rulesUsed {
		if len(rules) > 0 && !rules[r] {
			v.addf("vectors-rule", where, "rule %q used but not documented in RULES.md", r)
		}
	}
}

func loadVector(path, where string, v *violations) *dvector {
	raw, err := os.ReadFile(path)
	if err != nil {
		v.add("vector-read", where, err.Error())
		return nil
	}
	var vec dvector
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&vec); err != nil {
		v.add("vector-parse", where, err.Error())
		return nil
	}
	return &vec
}

func checkVector(vec *dvector, positive bool, dt, lang string, rules map[string]bool, where string, v *violations) {
	w := where + "/" + vec.ID
	if vec.Detector != dt {
		v.addf("vector-detector", w, "detector %q != directory %s", vec.Detector, dt)
	}
	if vec.Language != lang {
		v.addf("vector-lang", w, "language %q != directory %s", vec.Language, lang)
	}
	if vec.ID == "" {
		v.add("vector-id", w, "missing id")
	}
	if len(vec.Files) == 0 {
		v.add("vector-files", w, "no files")
	}
	treesDiffer := false
	for p, f := range vec.Files {
		if f.Before != f.After {
			treesDiffer = true
		}
		if strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
			v.addf("vector-path", w, "bad repo path %q", p)
		}
	}
	if !treesDiffer && len(vec.Events) == 0 {
		v.add("vector-change", w, "before == after and no tool events: the case exercises nothing")
	}
	for i, ev := range vec.Events {
		if ev.Name == "" {
			v.addf("vector-event", w, "tool_events[%d] has no name", i)
		}
	}
	if positive && len(vec.Expect) == 0 {
		v.add("vector-expect", w, "positive case has no expected hits")
	}
	if !positive && len(vec.Expect) != 0 {
		v.add("vector-expect", w, "negative case lists expected hits")
	}
	// commission-aware detectors need their commission inputs to fire
	if positive {
		switch vec.Detector {
		case "DT11":
			if vec.Commission == nil || len(vec.Commission.Territory) == 0 {
				v.add("vector-commission", w, "positive DT11 needs commission.territory")
			}
		case "DT12":
			if vec.Commission == nil || (len(vec.Commission.PlanFiles) == 0 && vec.Commission.PlanLines == 0) {
				v.add("vector-commission", w, "positive DT12 needs a backbrief plan")
			}
		}
	}
	// changed regions of the tree diff, per file
	changed := map[string]changedLines{}
	for p, f := range vec.Files {
		changed[p] = diffLines(f.Before, f.After)
	}
	for i, h := range vec.Expect {
		hw := fmt.Sprintf("%s expect[%d]", w, i)
		// a change can fire several detectors (03 §2): a .proto edit is both
		// DT2 format and DT3 contract, a //nolint is DT5+DT6 suppressed. The
		// hit may name any valid detector, but its vocabulary must match the
		// DETECTOR THAT PRODUCES IT.
		if _, ok := detectorCategory[h.Detector]; !ok {
			v.addf("vector-hit", hw, "hit detector %q unknown", h.Detector)
		}
		if detectorCategory[h.Detector] != h.Category {
			v.addf("vector-hit", hw, "category %q is not %s's category", h.Category, h.Detector)
		}
		if subs := detectorSubkinds[h.Detector]; len(subs) == 0 {
			if h.Subkind != "" {
				v.addf("vector-hit", hw, "%s has no subkinds; hit must leave subkind empty", h.Detector)
			}
		} else if !contains(subs, h.Subkind) {
			v.addf("vector-hit", hw, "subkind %q not in %s's subkind list", h.Subkind, h.Detector)
		}
		if h.Rule == "" {
			v.addf("vector-hit", hw, "rule id required (03 §2.1)")
		}
		switch h.Side {
		case "after", "before":
			f, ok := vec.Files[h.Path]
			if !ok {
				v.addf("vector-hit", hw, "path %q is not a file in the case", h.Path)
				continue
			}
			if h.Lines[0] < 1 || h.Lines[1] < h.Lines[0] {
				v.addf("vector-hit", hw, "bad line range %v", h.Lines)
				continue
			}
			text := f.After
			if h.Side == "before" {
				text = f.Before
			}
			lines := splitLines(text)
			if h.Lines[1] > len(lines) {
				v.addf("vector-hit", hw, "lines %v beyond %s file length %d", h.Lines, h.Side, len(lines))
				continue
			}
			span := strings.Join(lines[h.Lines[0]-1:h.Lines[1]], "\n")
			if h.Evidence == "" || !strings.Contains(span, h.Evidence) {
				v.addf("vector-hit", hw, "evidence text not found on lines %v of %s", h.Lines, h.Path)
			}
			// a hit's span must lie inside the changed region of its file
			cl := changed[h.Path]
			if h.Side == "after" && !cl.coversAfter(h.Lines[0], h.Lines[1]) {
				v.addf("vector-hit", hw, "lines %v are not inside the changed region of %s", h.Lines, h.Path)
			}
			if h.Side == "before" && !cl.coversBefore(h.Lines[0], h.Lines[1]) {
				v.addf("vector-hit", hw, "lines %v are not inside the removed region of %s", h.Lines, h.Path)
			}
		case "events":
			if len(vec.Events) == 0 {
				v.addf("vector-hit", hw, "side=events but the case has no tool_events")
			}
			found := false
			for _, ev := range vec.Events {
				if strings.Contains(fmt.Sprint(ev.Input), h.Evidence) && h.Evidence != "" {
					found = true
				}
			}
			if !found {
				v.addf("vector-hit", hw, "evidence %q found in no tool event input", h.Evidence)
			}
			// 03 §1.5: every hit carries file, line range and text digest.
			// Tool events are recorded as JSON lines of trajectory/events.jsonl,
			// so the span points at the event's own record — never an empty
			// path and [0,0].
			tf, ok := vec.Files[h.Path]
			if !ok || h.Path == "" {
				v.addf("vector-hit", hw,
					"event hit needs a trajectory file path (03 §1.5); got %q", h.Path)
			} else {
				tlines := splitLines(tf.After)
				if h.Lines[0] < 1 || h.Lines[1] > len(tlines) || h.Lines[1] < h.Lines[0] {
					v.addf("vector-hit", hw, "lines %v outside trajectory file of %d events", h.Lines, len(tlines))
				} else {
					span := strings.Join(tlines[h.Lines[0]-1:h.Lines[1]], "\n")
					if h.Evidence == "" || !strings.Contains(span, h.Evidence) {
						v.addf("vector-hit", hw, "evidence not on trajectory lines %v", h.Lines)
					}
				}
			}
		default:
			v.addf("vector-hit", hw, "side %q unknown", h.Side)
		}
		// 03 §1.5 + F4: a deleted_test hit's span covers the whole removed
		// test function, not just its header.
		if h.Subkind == "deleted_test" && h.Side == "before" {
			if f, ok := vec.Files[h.Path]; ok {
				lo, hi := removedTestExtent(f.Before, vec.Language, h.Lines[0])
				if lo > 0 && (lo != h.Lines[0] || hi != h.Lines[1]) {
					v.addf("vector-hit", hw,
						"deleted_test span %v must cover the whole removed test [%d,%d]",
						h.Lines, lo, hi)
				}
			}
		}
	}
	// F3 completeness: every mechanical hit must be expected. On negatives
	// (expect: []) the same fires prove the case is mislabeled.
	checkCrossFires(vec, w, v)
	_ = rules
}

// removedTestExtent returns the [lo,hi] line range of the test function
// whose declaration starts at start (1-based) in text; (0,0) when no test
// declaration is found there.
func removedTestExtent(text, lang string, start int) (int, int) {
	lines := splitLines(text)
	if start < 1 || start > len(lines) {
		return 0, 0
	}
	decl := lines[start-1]
	switch lang {
	case "go", "typescript":
		if !strings.Contains(decl, "func Test") &&
			!strings.Contains(decl, "func Benchmark") &&
			!strings.Contains(decl, "test(") && !strings.Contains(decl, "it(") &&
			!strings.Contains(decl, "describe(") &&
			!testDeclAtStartTS(decl) {
			return 0, 0
		}
		depth := 0
		end := start
		opened := false
		for i := start - 1; i < len(lines); i++ {
			for _, r := range lines[i] {
				switch r {
				case '{':
					depth++
					opened = true
				case '}':
					depth--
				}
			}
			end = i + 1
			if opened && depth <= 0 {
				// a one-line test('x', () => {}); ends on the same line
				return start, end
			}
			// arrow-body tests close with `});` even at depth 0
			if opened && depth == 0 {
				return start, end
			}
		}
		return start, end
	default: // python: ends at the next top-level def/decorator or EOF
		if !strings.HasPrefix(decl, "def test") && !strings.HasPrefix(decl, "async def test") {
			return 0, 0
		}
		end := len(lines)
		for i := start; i < len(lines); i++ {
			l := lines[i]
			if strings.TrimSpace(l) == "" {
				continue
			}
			if !strings.HasPrefix(l, " ") && !strings.HasPrefix(l, "\t") {
				end = i
				break
			}
		}
		return start, end
	}
}

func testDeclAtStartTS(decl string) bool {
	for _, kw := range []string{"test.skip(", "test.only(", "it.skip(", "it.only(",
		"describe.skip(", "describe.only(", "xit(", "xtest(", "xdescribe("} {
		if strings.Contains(decl, kw) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// cross-fire completeness (F3): a vector expects every hit 03 §2 produces
// for its change. The recognizer covers the mechanical rules only — every
// rule below is a direct reading of the spec text, not a judgement call.
// The same fires make a negative case mislabeled when they appear there.
// ---------------------------------------------------------------------------

var (
	cfMigDir    = regexp.MustCompile(`(^|/)(migrations|alembic/versions|db/migrations|prisma/migrations)/`)
	cfDDL       = regexp.MustCompile(`(?i)\b(create|alter|drop)\s+table\b|\b(add|drop)\s+column\b|\b(create|drop)\s+index\b`)
	cfDestruct  = regexp.MustCompile(`(?i)\b(drop|truncate)\s+(table|index|column|view|sequence|schema|database|trigger)\b|\bdelete\s+from\s+[a-z_][a-z0-9_.]*\s*;?\s*$`)
	cfAuthSeg   = regexp.MustCompile(`(^|/)(auth|login|session|token|permission|acl|rbac|secret|credential|crypto)(\.|/|$)`)
	cfSuppress5 = []string{"//nolint", "@ts-ignore", "@ts-expect-error", "# type: ignore", "#[allow("}
	cfSuppress6 = []string{"noqa", "eslint-disable"}
	cfDataFile  = regexp.MustCompile(`\.(csv|parquet|sqlite|db)$|(^|/)(data|fixtures)/`)
	cfBadCmd    = regexp.MustCompile(`\brm\s+-[a-z]*r|\bgit\s+clean\b|\bgit\s+reset\s+--hard|\bgit\s+push\b.*(--force|-f)\b|\bgit\s+push\s+-f\b`)
	cfHost      = regexp.MustCompile(`(?:https?|wss|ftp)://([a-zA-Z0-9.-]+\.[a-zA-Z]{2,}|localhost|[0-9.]+)|\b(?:curl|wget|ping|nc|ncat|ssh|scp|fetch|host|dig|nslookup|telnet|git\s+fetch|git\s+clone|git\s+ls-remote|git\s+pull|git\s+push)\s+(?:-+[^\s]+\s+)*(?:[^\s:/@]+@)?([a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+|localhost|[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)`)
)

func checkCrossFires(vec *dvector, w string, v *violations) {
	has := func(det, sub string) bool {
		for _, h := range vec.Expect {
			if h.Detector == det && (sub == "" || h.Subkind == sub) {
				return true
			}
		}
		return false
	}
	reported := map[string]bool{}
	need := func(det, cat, sub, why string) {
		if has(det, sub) {
			return
		}
		k := det + "\x00" + sub
		if reported[k] {
			return
		}
		reported[k] = true
		v.addf("vector-incomplete", w,
			"change also fires %s %s (%s); a vector expects every hit 03 §2 produces", det, sub, why)
	}
	inPlan := func(path string) bool {
		return vec.Commission != nil &&
			matchesTerritory(vec.Commission.PlanFiles, path)
	}
	for path, f := range vec.Files {
		if f.Before == f.After || exemptPath(path) ||
			strings.HasPrefix(path, "trajectory/") {
			continue
		}
		// DT2: file changed under a recognised migration directory
		if cfMigDir.MatchString(path) {
			need("DT2", "schema", "migration", "path sits under a migration directory")
		}
		// DT2 format files
		if strings.HasSuffix(path, ".proto") || strings.HasSuffix(path, ".avsc") ||
			strings.HasSuffix(path, ".schema.json") || strings.HasSuffix(path, "schema.prisma") {
			need("DT2", "schema", "format", "format file changed")
		}
		// DT3 contract files
		if strings.HasSuffix(path, ".proto") || strings.HasSuffix(path, ".graphql") ||
			contractYAML(path) {
			need("DT3", "interface", "", "contract file changed")
		}
		// DT9 touched: authority path segment changed
		if cfAuthSeg.MatchString(path) {
			need("DT9", "authority", "touched", "path has an authority segment")
		}
		// DT10 file: tracked data file deleted
		if f.After == "" && cfDataFile.MatchString(path) {
			need("DT10", "deletion", "file", "tracked data file deleted")
		}
		// DT11 territory: path outside every glob
		if vec.Commission != nil && len(vec.Commission.Territory) > 0 &&
			!matchesTerritory(vec.Commission.Territory, path) {
			need("DT11", "territory", "", "changed path matches no territory glob")
		}
		// DT12 unplanned-file: changed path not in the backbrief plan
		if vec.Commission != nil && len(vec.Commission.PlanFiles) > 0 &&
			!inPlan(path) && !exemptPath(path) {
			need("DT12", "scope", "unplanned-file", "changed path not in plan_files")
		}
		doc := cfDocFile(path)
		// added-line scans (DDL / destructive SQL / suppressions / hosts)
		for _, ln := range addedLines(f.Before, f.After) {
			trim := strings.TrimSpace(ln)
			comment := strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") ||
				strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "/*") ||
				strings.HasPrefix(trim, "*")
			if strings.HasSuffix(path, ".sql") && !comment && cfDDL.MatchString(ln) {
				need("DT2", "schema", "ddl", ".sql hunk adds DDL")
			}
			if !comment && (strings.HasSuffix(path, ".sql") || inStringLit(ln)) &&
				cfDestruct.MatchString(ln) {
				need("DT10", "deletion", "sql", "added line adds unguarded delete")
			}
			for _, tok := range cfSuppress5 {
				if strings.Contains(ln, tok) {
					need("DT5", "type-strength", "suppressed", "checker suppression added")
					need("DT6", "guardrail", "suppressed", "inline lint suppression added")
				}
			}
			for _, tok := range cfSuppress6 {
				if strings.Contains(ln, tok) {
					need("DT6", "guardrail", "suppressed", "inline lint suppression added")
				}
			}
			// A hostname inside a comment or a doc file is not a host literal
			// (03 §2 DT8, near miss "comment-hostname").
			if !comment && !doc {
				if m := cfHost.FindStringSubmatch(ln); m != nil {
					host := m[1]
					if host == "" {
						host = m[2]
					}
					if host != "" && !hostInTree(vec, host) {
						need("DT8", "outbound", "code", "added line carries a host absent from the base tree")
					}
				}
			}
		}
	}
	for _, ev := range vec.Events {
		cmd := fmt.Sprint(ev.Input)
		if cfBadCmd.MatchString(cmd) {
			need("DT10", "deletion", "command", "tool event ran a destructive command")
		}
		if vec.Commission != nil {
			if m := cfHost.FindStringSubmatch(cmd); m != nil {
				host := m[1]
				if host == "" {
					host = m[2]
				}
				if host != "" && !contains(vec.Commission.AllowedHosts, host) {
					need("DT8", "outbound", "agent", "tool event contacted a host outside allowed_hosts")
				}
			}
		}
	}
	// DT12 paired subkinds: a new top-level file the plan does not name is
	// also an unplanned-file, and a new file in a new top-level directory is
	// also new-top-level when the plan names it.
	if vec.Commission != nil {
		for _, h := range vec.Expect {
			if h.Detector != "DT12" {
				continue
			}
			if h.Subkind == "new-top-level" && len(vec.Commission.PlanFiles) > 0 &&
				!inPlan(h.Path) {
				need("DT12", "scope", "unplanned-file", "new top-level path is also not in the plan")
			}
			if h.Subkind == "oversize" && len(vec.Commission.PlanFiles) > 0 &&
				!inPlan(h.Path) && !exemptPath(h.Path) {
				need("DT12", "scope", "unplanned-file", "oversize path is also not in the plan")
			}
		}
	}
}

// cfDocFile: prose files where a URL is documentation, not a host literal.
func cfDocFile(p string) bool {
	for _, ext := range []string{".md", ".txt", ".rst", ".adoc"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

// contractYAML matches the openapi*.yaml|json contract shapes.
func contractYAML(p string) bool {
	base := filepath.Base(p)
	return strings.HasPrefix(base, "openapi") &&
		(strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".json"))
}

// matchesTerritory implements the glob subset used in the corpus: "**"
// matches everything, "dir/**" matches any path under dir, "dir/*" matches
// one level, and a literal path matches itself.
func matchesTerritory(globs []string, path string) bool {
	for _, g := range globs {
		switch {
		case g == "**":
			return true
		case strings.HasSuffix(g, "/**"):
			if strings.HasPrefix(path, strings.TrimSuffix(g, "**")) {
				return true
			}
		case strings.HasSuffix(g, "/*"):
			pref := strings.TrimSuffix(g, "*")
			if strings.HasPrefix(path, pref) && !strings.Contains(strings.TrimPrefix(path, pref), "/") {
				return true
			}
		case g == path:
			return true
		}
	}
	return false
}

// exemptPath covers the 03 §2.2 generated/vendor paths that never reach a
// detector.
func exemptPath(p string) bool {
	for _, seg := range []string{"vendor/", "node_modules/", "dist/", "build/",
		"target/", "__pycache__/", ".venv/", ".git/"} {
		if strings.HasPrefix(p, seg) || strings.Contains(p, "/"+seg) {
			return true
		}
	}
	return false
}

// addedLines returns the lines appearing in after that are not in before
// (a multiset difference, aligned loosely — adequate for the mechanical
// recognizers above).
func addedLines(before, after string) []string {
	bm := map[string]int{}
	for _, l := range splitLines(before) {
		bm[l]++
	}
	var out []string
	for _, l := range splitLines(after) {
		if bm[l] > 0 {
			bm[l]--
			continue
		}
		out = append(out, l)
	}
	return out
}

func inStringLit(ln string) bool {
	return strings.Contains(ln, `"`) || strings.Contains(ln, `'`)
}

func hostInTree(vec *dvector, host string) bool {
	for _, f := range vec.Files {
		if strings.Contains(f.Before, host) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// line diff for the changed-region invariant
// ---------------------------------------------------------------------------

// changedLines records which line ranges differ between before and after.
// before and after share a common prefix/suffix; the middle is the change.
type changedLines struct {
	bLo, bHi int // 1-based inclusive range removed in before (empty: bLo>bHi)
	aLo, aHi int // 1-based inclusive range added in after
}

func diffLines(before, after string) changedLines {
	b, a := splitLines(before), splitLines(after)
	p := 0
	for p < len(b) && p < len(a) && b[p] == a[p] {
		p++
	}
	sB, sA := len(b), len(a)
	for sB > p && sA > p && b[sB-1] == a[sA-1] {
		sB--
		sA--
	}
	return changedLines{bLo: p + 1, bHi: sB, aLo: p + 1, aHi: sA}
}

// hunkContext is the number of unchanged lines a diff hunk carries on
// each side of a change; a hit's span may sit on those context lines
// (03 §1.5: the span anchors where a reviewer looks in the hunk).
const hunkContext = 3

func (c changedLines) coversAfter(lo, hi int) bool {
	if c.aHi == 0 {
		return false
	}
	return lo >= c.aLo-hunkContext && hi <= c.aHi+hunkContext
}

func (c changedLines) coversBefore(lo, hi int) bool {
	if c.bHi == 0 {
		return false
	}
	return lo >= c.bLo-hunkContext && hi <= c.bHi+hunkContext
}

// ---------------------------------------------------------------------------
// case distinctness (F1): two vectors are the same case when their inputs
// match after normalising identifiers, digits and string contents
// ---------------------------------------------------------------------------

var (
	normIdent  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	normDigits = regexp.MustCompile(`[0-9]+`)
	normStrDQ  = regexp.MustCompile(`"[^"\n]*"`)
	normStrSQ  = regexp.MustCompile(`'[^'\n]*'`)
	normWS     = regexp.MustCompile(`[ \t]+`)
)

// normKeywords are language/config keywords kept literal so that different
// constructs stay distinct; every other identifier token becomes @.
var normKeywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`
		func var const type struct interface map chan range go defer if else for
		switch case default return break continue fallthrough select package import
		def class lambda return if elif else for while try except finally raise
		with as import from pass break continue global nonlocal yield assert del
		is in not and or True False None async await
		function return if else for while do switch case default break continue
		try catch finally throw new delete typeof instanceof in of var let const
		class extends super this import export from async await yield static get set
		void null undefined true false
		create alter drop table column index add select insert update delete from
		where values set into truncate grant revoke view trigger sequence
		primary key foreign references constraint unique check not null default
	`) {
		normKeywords[k] = true
	}
}

func normText(s string) string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = normStrDQ.ReplaceAllString(ln, `""`)
		ln = normStrSQ.ReplaceAllString(ln, `''`)
		ln = normIdent.ReplaceAllStringFunc(ln, func(tok string) string {
			if k := strings.ToLower(tok); normKeywords[k] {
				return k
			}
			return "@"
		})
		ln = normDigits.ReplaceAllString(ln, "#")
		ln = normWS.ReplaceAllString(ln, " ")
		out = append(out, strings.TrimSpace(ln))
	}
	return strings.Join(out, "\n")
}

// normPath normalises each path segment like content but preserves the
// file extension, so migrations/0003_x.sql and migrations/0004_y.sql are the
// same case while migrations/x.sql and data/x.csv differ.
func normPath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		ext := ""
		if j := strings.LastIndex(s, "."); j >= 0 {
			ext = s[j:]
			s = s[:j]
		}
		s = normIdent.ReplaceAllStringFunc(s, func(tok string) string {
			if k := strings.ToLower(tok); normKeywords[k] {
				return k
			}
			return "@"
		})
		segs[i] = normDigits.ReplaceAllString(s, "#") + ext
	}
	return strings.Join(segs, "/")
}

// digitDirs captures the direction of digit changes between before/after
// (a threshold falling vs rising is a different case even after digits
// normalise to #).
func digitDirs(before, after string) string {
	bl := splitLines(before)
	al := splitLines(after)
	n := len(bl)
	if len(al) < n {
		n = len(al)
	}
	var dirs []string
	for i := 0; i < n; i++ {
		bd := normDigits.FindAllString(bl[i], -1)
		ad := normDigits.FindAllString(al[i], -1)
		if len(bd) == 0 || len(bd) != len(ad) {
			continue
		}
		for j := range bd {
			var bv, av int
			fmt.Sscanf(bd[j], "%d", &bv)
			fmt.Sscanf(ad[j], "%d", &av)
			switch {
			case av > bv:
				dirs = append(dirs, "+")
			case av < bv:
				dirs = append(dirs, "-")
			}
		}
	}
	return strings.Join(dirs, "")
}

func vectorSignature(vec *dvector) string {
	var b strings.Builder
	paths := make([]string, 0, len(vec.Files))
	for p := range vec.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := vec.Files[p]
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%s\x00", normPath(p),
			normText(f.Before), normText(f.After), digitDirs(f.Before, f.After))
	}
	for _, ev := range vec.Events {
		inp, _ := json.Marshal(ev.Input)
		fmt.Fprintf(&b, "EV:%s\x00", normText(ev.Name+string(inp)+ev.Outcome))
	}
	if vec.Commission != nil {
		c, _ := json.Marshal(vec.Commission)
		fmt.Fprintf(&b, "CM:%s\x00", normText(string(c)))
	}
	return b.String()
}
