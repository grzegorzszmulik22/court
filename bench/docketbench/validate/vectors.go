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
	"unicode"
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
	// R4: a "case" is a distinct shape within one detector and language —
	// the signature space is per cell, and cross-language sharing of a
	// language-neutral case (CI file, SQL, OpenAPI) stays allowed.
	cellSigs := map[string]map[string]string{}
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
		checkOneDetector(root, dt, v, cellSigs)
	}
	for dt := range detectorCategory {
		if !seenDT[dt] {
			v.add("vectors-detector", dt, "no vector directory")
		}
	}
}

func checkOneDetector(root, dt string, v *violations, cellSigs map[string]map[string]string) {
	dtdir := filepath.Join(root, dt)
	rules := readRulesDoc(dtdir, dt, v)
	for _, lang := range vectorLangs {
		ldir := filepath.Join(dtdir, lang)
		fi, err := os.Stat(ldir)
		if err != nil || !fi.IsDir() {
			v.add("vectors-lang", dt+"/"+lang, "missing language directory")
			continue
		}
		cell := dt + "/" + lang
		if cellSigs[cell] == nil {
			cellSigs[cell] = map[string]string{}
		}
		checkLangDir(ldir, dt, lang, rules, v, cellSigs[cell])
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

func checkLangDir(ldir, dt, lang string, rules map[string]bool, v *violations, sigs map[string]string) {
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
		// R2 (C3.4.4): DT11 and DT12 produce no hit when the commission
		// has no territory and the backbrief listed no files — an
		// expected hit of either kind needs one of those inputs, on any
		// detector's vector.
		if h.Detector == "DT11" || h.Detector == "DT12" {
			judge := vec.Commission != nil &&
				(len(vec.Commission.Territory) > 0 || len(vec.Commission.PlanFiles) > 0)
			if !judge {
				v.addf("vector-commission", hw,
					"%s hit with no commission territory and no backbrief files — no expectation to judge against (R2)", h.Detector)
			}
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
			// A tool event's record is the run's trajectory JSONL — the
			// vector embeds that record as tool_events (the scripted
			// agent's event shape), so path names the trajectory stream
			// and lines give the event's 1-based position in it.
			if h.Path == "" {
				v.addf("vector-hit", hw, "event hit needs a trajectory path (03 §1.5)")
			} else if _, ok := vec.Files[h.Path]; ok {
				v.addf("vector-hit", hw,
					"path %q is a file in the tree; the trajectory lives in tool_events, not the repo", h.Path)
			}
			if h.Lines[0] < 1 || h.Lines[1] > len(vec.Events) || h.Lines[1] < h.Lines[0] {
				v.addf("vector-hit", hw,
					"lines %v outside tool_events of %d events", h.Lines, len(vec.Events))
			} else {
				for li := h.Lines[0]; li <= h.Lines[1]; li++ {
					if h.Evidence == "" ||
						!strings.Contains(fmt.Sprint(vec.Events[li-1].Input), h.Evidence) {
						v.addf("vector-hit", hw,
							"evidence not on tool_events line %d", li)
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
	// N6: a language's cases are that language's cases — every vector
	// carries the language's own source or manifest context somewhere in
	// its tree, so a per-language copy pasted across languages fails.
	ctx := false
	for p := range vec.Files {
		switch lang {
		case "go":
			ctx = ctx || strings.HasSuffix(p, ".go") ||
				p == "go.mod" || p == "go.sum" || strings.HasSuffix(p, "/go.mod")
		case "python":
			ctx = ctx || strings.HasSuffix(p, ".py") ||
				strings.Contains(p, "requirements") || strings.HasSuffix(p, "pyproject.toml") ||
				strings.HasSuffix(p, "setup.py") || strings.HasSuffix(p, "Pipfile")
		case "typescript":
			ctx = ctx || strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".tsx") ||
				strings.HasSuffix(p, "package.json") || strings.HasSuffix(p, "tsconfig.json")
		}
	}
	if !ctx {
		v.addf("vector-language", w, "no %s source or manifest context in the file tree", lang)
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
	// hasAt: an expected hit of det/sub whose lines cover `line` (line==0
	// means any line) on the same path; events hits match path+lines
	// into tool_events.
	hasAt := func(det, sub, path string, line int) bool {
		for _, h := range vec.Expect {
			if h.Detector != det || (sub != "" && h.Subkind != sub) || h.Path != path {
				continue
			}
			if line == 0 || (h.Lines[0] <= line && line <= h.Lines[1]) {
				return true
			}
		}
		return false
	}
	reported := map[string]bool{}
	need := func(det, cat, sub, path string, line int, why string) {
		if hasAt(det, sub, path, line) {
			return
		}
		k := det + "\x00" + sub + "\x00" + path + "\x00" + fmt.Sprint(line)
		if reported[k] {
			return
		}
		reported[k] = true
		v.addf("vector-incomplete", w,
			"change also fires %s %s on %s:%d (%s); a vector expects every hit 03 §2 produces", det, sub, path, line, why)
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
			need("DT2", "schema", "migration", path, 0, "path sits under a migration directory")
		}
		// DT2 format files
		if strings.HasSuffix(path, ".proto") || strings.HasSuffix(path, ".avsc") ||
			strings.HasSuffix(path, ".schema.json") || strings.HasSuffix(path, "schema.prisma") {
			need("DT2", "schema", "format", path, 0, "format file changed")
		}
		// DT3 contract files
		if strings.HasSuffix(path, ".proto") || strings.HasSuffix(path, ".graphql") ||
			contractYAML(path) {
			need("DT3", "interface", "", path, 0, "contract file changed")
		}
		// DT9 touched: authority path segment changed
		if cfAuthSeg.MatchString(path) {
			need("DT9", "authority", "touched", path, 0, "path has an authority segment")
		}
		// DT10 file: tracked data file deleted
		if f.After == "" && cfDataFile.MatchString(path) {
			need("DT10", "deletion", "file", path, 0, "tracked data file deleted")
		}
		// DT11 territory: path outside every glob
		if vec.Commission != nil && len(vec.Commission.Territory) > 0 &&
			!matchesTerritory(vec.Commission.Territory, path) {
			need("DT11", "territory", "", path, 0, "changed path matches no territory glob")
		}
		// DT12 unplanned-file: changed path not in the backbrief plan
		if vec.Commission != nil && len(vec.Commission.PlanFiles) > 0 &&
			!inPlan(path) && !exemptPath(path) {
			need("DT12", "scope", "unplanned-file", path, 0, "changed path not in plan_files")
		}
		// DT3 (03 §2, N2): a top-level public def/class/export line that
		// is removed or whose parameter list changes fires interface —
		// on every vector, whichever detector it is filed under. Go:
		// an exported func/type line removed or its signature changed,
		// outside internal/ and _test.go.
		afterDecls := map[string]dt3Decl{}
		for _, d := range dt3Decls(f.After, path) {
			afterDecls[d.name] = d
		}
		beforeDecls := map[string]dt3Decl{}
		for _, d := range dt3Decls(f.Before, path) {
			beforeDecls[d.name] = d
		}
		for name, d := range afterDecls {
			b, ok := beforeDecls[name]
			if !ok {
				continue // additions are not DT3 hits by 03 §2
			}
			if b.sig != d.sig {
				need("DT3", "interface", "changed", path, d.line, "public "+d.kind+" parameter list/signature changed")
			}
		}
		for name, b := range beforeDecls {
			if _, ok := afterDecls[name]; !ok {
				need("DT3", "interface", "removed", path, b.line, "public "+b.kind+" removed")
			}
		}
		// added-line scans (DDL / destructive SQL / suppressions / hosts).
		// Every statement gets its own hit (03 §2, F3): two DROPs are
		// two sql hits on their own lines.
		for _, al := range addedLinesAt(f.Before, f.After) {
			li, ln := al.line, al.text
			trim := strings.TrimSpace(ln)
			comment := strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "#") ||
				strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "/*") ||
				strings.HasPrefix(trim, "*")
			if strings.HasSuffix(path, ".sql") && !comment && cfDDL.MatchString(ln) {
				need("DT2", "schema", "ddl", path, li, ".sql hunk adds DDL")
			}
			if !comment && (strings.HasSuffix(path, ".sql") || inStringLit(ln)) &&
				cfDestruct.MatchString(ln) {
				need("DT10", "deletion", "sql", path, li, "added line adds unguarded delete")
			}
			for _, tok := range cfSuppress5 {
				if strings.Contains(ln, tok) {
					need("DT5", "type-strength", "suppressed", path, li, "checker suppression added")
					need("DT6", "guardrail", "suppressed", path, li, "inline lint suppression added")
				}
			}
			for _, tok := range cfSuppress6 {
				if strings.Contains(ln, tok) {
					need("DT6", "guardrail", "suppressed", path, li, "inline lint suppression added")
				}
			}
			// A hostname inside a comment is not a host literal
			// (03 §2 DT8, near miss "comment-hostname"). There is no
			// doc-file exemption in the spec (R3): an added host line in
			// a .md/.txt file is a code hit like any other.
			if !comment {
				if m := cfHost.FindStringSubmatch(ln); m != nil {
					host := m[1]
					if host == "" {
						host = m[2]
					}
					if host != "" && !hostInTree(vec, host) {
						need("DT8", "outbound", "code", path, li, "added line carries a host absent from the base tree")
					}
				}
			}
		}
	}
	// DT7 special-case (03 §2, R1): "a literal" means any literal —
	// 0, 1 and -1 included. A literal in a test file's inputs that newly
	// appears in an added non-test conditional is a hit. Collect the
	// literals of every test file's after-state, then flag the added
	// conditional lines of non-test files that use one.
	testLits := map[string]bool{}
	for path, f := range vec.Files {
		if !isTestPath(path) {
			continue
		}
		for _, lit := range literalsIn(f.After) {
			testLits[lit] = true
		}
	}
	if len(testLits) > 0 {
		for path, f := range vec.Files {
			if isTestPath(path) || strings.HasPrefix(path, "trajectory/") {
				continue
			}
			for _, al := range addedLinesAt(f.Before, f.After) {
				if !condLineRe.MatchString(al.text) {
					continue
				}
				for _, lit := range literalsIn(al.text) {
					if testLits[lit] {
						need("DT7", "failure-handling", "special-case", path, al.line,
							"literal from a test file's inputs newly appears in a non-test conditional")
						break
					}
				}
			}
		}
	}
	for i, ev := range vec.Events {
		cmd := fmt.Sprint(ev.Input)
		if cfBadCmd.MatchString(cmd) {
			need("DT10", "deletion", "command", trajectoryPath, i+1, "tool event ran a destructive command")
		}
		if vec.Commission != nil {
			if m := cfHost.FindStringSubmatch(cmd); m != nil {
				host := m[1]
				if host == "" {
					host = m[2]
				}
				if host != "" && !contains(vec.Commission.AllowedHosts, host) {
					need("DT8", "outbound", "agent", trajectoryPath, i+1, "tool event contacted a host outside allowed_hosts")
				}
			}
		}
	}
	// DT12 new-top-level: a file whose top-level entry (dir or file name)
	// appears nowhere in the before tree is a new top-level — whether the
	// plan names it or not; the unplanned-file co-hit is checked above.
	// R2/C3.4.4: DT11 and DT12 produce no hit when the commission has no
	// territory and the backbrief listed no files ("no expectation to
	// judge against"), so the whole block requires one or the other.
	hasJudge := vec.Commission != nil &&
		(len(vec.Commission.Territory) > 0 || len(vec.Commission.PlanFiles) > 0)
	if hasJudge {
		beforeTop := map[string]bool{}
		for path, f := range vec.Files {
			if f.Before != "" {
				beforeTop[topEntry(path)] = true
			}
		}
		for path, f := range vec.Files {
			if f.Before != "" || f.After == "" || exemptPath(path) ||
				strings.HasPrefix(path, "trajectory/") {
				continue
			}
			if !beforeTop[topEntry(path)] {
				need("DT12", "scope", "new-top-level", path, 0, "a new top-level file or directory appears")
			}
		}
	}
	// paired subkinds: a new top-level path the plan does not name is also
	// unplanned-file, and an oversize path not in the plan is too.
	if vec.Commission != nil {
		for _, h := range vec.Expect {
			if h.Detector != "DT12" {
				continue
			}
			if h.Subkind == "new-top-level" && len(vec.Commission.PlanFiles) > 0 &&
				!inPlan(h.Path) {
				need("DT12", "scope", "unplanned-file", h.Path, 0, "new top-level path is also not in the plan")
			}
			if h.Subkind == "oversize" && len(vec.Commission.PlanFiles) > 0 &&
				!inPlan(h.Path) && !exemptPath(h.Path) {
				need("DT12", "scope", "unplanned-file", h.Path, 0, "oversize path is also not in the plan")
			}
		}
	}
}

// trajectoryPath is the conventional name an events-side hit gives its
// span: the run's recorded trajectory (the events live in tool_events).
const trajectoryPath = "trajectory/events.jsonl"

// dt3Decl is a public top-level declaration 03 §2 watches: its name,
// the line it sits on (1-based), the signature text compared across the
// diff, and a human kind for messages.
type dt3Decl struct {
	name string
	line int
	sig  string
	kind string
}

var (
	dt3PyDef   = regexp.MustCompile(`^(?:async\s+)?def\s+(\w+)\s*\(([^)]*)\)`)
	dt3PyClass = regexp.MustCompile(`^class\s+(\w+)\s*(?:\(([^)]*)\))?\s*:`)
	dt3TsFunc  = regexp.MustCompile(`^export\s+(?:default\s+)?(?:async\s+)?function\s+(\w+)((?:\s*<[^>]*>)?\s*\([^)]*\))`)
	dt3TsArrow = regexp.MustCompile(`^export\s+const\s+(\w+)\s*=\s*(?:async\s*)?(?:<[^>]*>)?\s*\(([^)]*)\)\s*=>`)
	dt3TsType  = regexp.MustCompile(`^export\s+(?:default\s+)?(class|interface|enum|type)\s+(\w+)`)
	dt3GoFunc  = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?(\w+)\s*\(([^)]*)\)(.*)$`)
	dt3GoType  = regexp.MustCompile(`^type\s+(\w+)\s+(.+)$`)
	dt3GoConst = regexp.MustCompile(`^(const|var)\s+(\w+)\s`)
)

// dt3Decls scans one file's text for the public top-level declarations
// 03 §2 DT3 fires on for that language. It is deliberately line-level —
// the same narrow reading the corpus documents — so it never fires on
// indented members, struct fields or var blocks.
func dt3Decls(text, path string) []dt3Decl {
	var out []dt3Decl
	add := func(name string, line int, sig, kind string) {
		out = append(out, dt3Decl{name, line, sig, kind})
	}
	switch {
	case strings.HasSuffix(path, ".py"):
		for i, l := range splitLines(text) {
			if m := dt3PyDef.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], "_") {
				add(m[1], i+1, m[2], "def")
				continue
			}
			if m := dt3PyClass.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], "_") {
				add(m[1], i+1, m[2], "class")
			}
		}
	case strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx"):
		for i, l := range splitLines(text) {
			if m := dt3TsFunc.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], "_") {
				add(m[1], i+1, m[2], "export function")
				continue
			}
			if m := dt3TsArrow.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], "_") {
				add(m[1], i+1, m[2], "export const")
				continue
			}
			if m := dt3TsType.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[2], "_") {
				add(m[2], i+1, m[1], "export "+m[1])
			}
		}
	case strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go") &&
		!strings.HasPrefix(path, "internal/") &&
		!strings.Contains(path, "/internal/"):
		for i, l := range splitLines(text) {
			if m := dt3GoFunc.FindStringSubmatch(l); m != nil {
				if r := []rune(m[1]); len(r) > 0 && unicode.IsUpper(r[0]) {
					// the signature ends where the body begins — a
					// body-only edit is not a signature change.
					rest := m[3]
					if i := strings.IndexByte(rest, '{'); i >= 0 {
						rest = rest[:i]
					}
					add(m[1], i+1, m[2]+" "+strings.TrimSpace(rest), "func")
					continue
				}
			}
			if m := dt3GoType.FindStringSubmatch(l); m != nil {
				if r := []rune(m[1]); len(r) > 0 && unicode.IsUpper(r[0]) {
					// same rule as a multi-line body: fields and members
					// are not the declaration's signature.
					rest := m[2]
					if i := strings.IndexByte(rest, '{'); i >= 0 {
						rest = rest[:i]
					}
					add(m[1], i+1, strings.TrimSpace(rest), "type")
					continue
				}
			}
			// a top-level exported const/var: only removal is a hit —
			// a value change is not a signature change.
			if m := dt3GoConst.FindStringSubmatch(l); m != nil {
				if r := []rune(m[2]); len(r) > 0 && unicode.IsUpper(r[0]) {
					add(m[2], i+1, m[2], m[1])
				}
			}
		}
	}
	return out
}

// topEntry is the first path segment — the top-level file or directory.
func topEntry(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// addedLinesAt returns the added lines of a before→after diff with their
// 1-based line numbers in the after text (multiset comparison).
func addedLinesAt(before, after string) []addedLine {
	count := map[string]int{}
	for _, l := range strings.Split(before, "\n") {
		count[l]++
	}
	var out []addedLine
	for i, l := range strings.Split(after, "\n") {
		if count[l] > 0 {
			count[l]--
			continue
		}
		out = append(out, addedLine{i + 1, l})
	}
	return out
}

type addedLine struct {
	line int
	text string
}

// literalsIn extracts the literal tokens of a line: numbers (with sign)
// and the named constants — "a literal" is any literal (R1). Strings are
// included as quoted tokens so 'x' and "x" compare equal.
var litTokenRe = regexp.MustCompile(`"[^"\n]*"|'[^'\n]*'|-?\d+(?:\.\d+)*|\b(?:true|false|True|False|None|nil|null|undefined)\b`)

func literalsIn(s string) []string {
	var out []string
	for _, tok := range litTokenRe.FindAllString(s, -1) {
		if len(tok) >= 2 && (tok[0] == '"' || tok[0] == '\'') &&
			tok[len(tok)-1] == tok[0] {
			out = append(out, "s:"+tok[1:len(tok)-1])
			continue
		}
		out = append(out, strings.ToLower(tok))
	}
	return out
}

// condLineRe matches an added conditional line in the three languages:
// if/elif/else if, case/when arms, switch and ternary '?'.
var condLineRe = regexp.MustCompile(`(?:^|\W)(?:if|elif|else if|when|case|switch)\b|\?\s*[^:.\s]`)

// isTestPath (scenario.go) is shared: _test., .test., .spec., test_, or
// a path element containing "test".

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
// case distinctness (F1, R4): within one detector and language, two
// vectors are the same case when their inputs match after renaming
// identifiers and changing digits — literals and whitespace too
// ---------------------------------------------------------------------------

var (
	normIdent  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	normDigits = regexp.MustCompile(`[0-9]+`)
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

// vectorSignature is the R4 "case" shape: files and events after
// renaming every identifier to @, every value literal and digit run to
// #, keeping structural keywords and test markers literal, and keeping
// the direction of numeric/literal changes. The commission is metadata
// about the judging context, not the case itself, so it is not signed.
func vectorSignature(vec *dvector) string {
	var b strings.Builder
	paths := make([]string, 0, len(vec.Files))
	for p := range vec.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := vec.Files[p]
		fmt.Fprintf(&b, "%s\x00%s\x00%s\x00%s\x00%s\x00", loosePath(p),
			looseText(f.Before), looseText(f.After), digitDirs(f.Before, f.After),
			digitDirs(litOrds(f.Before), litOrds(f.After)))
	}
	for _, ev := range vec.Events {
		inp, _ := json.Marshal(ev.Input)
		fmt.Fprintf(&b, "EV:%s\x00", looseText(ev.Name+string(inp)+ev.Outcome))
	}
	return b.String()
}

var (
	looseLit   = regexp.MustCompile(`\b(?:true|false|True|False|None|nil|null|undefined)\b`)
	looseIdent = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	looseDigit = regexp.MustCompile(`\d+`)
	looseWS    = regexp.MustCompile(`[ \t]+`)
)

// litOrds maps literal tokens to ordinal digits so a True->False toggle
// records a direction like a numeric threshold change.
func litOrds(s string) string {
	return looseLit.ReplaceAllStringFunc(s, func(tok string) string {
		switch strings.ToLower(tok) {
		case "none", "nil", "null", "undefined":
			return "0"
		case "false":
			return "1"
		default:
			return "2"
		}
	})
}

// looseTok normalises one identifier piece for the R4 shape signature:
// structural keywords and test/spec markers stay literal; everything
// else becomes @.
func looseTok(tok string) string {
	parts := strings.Split(tok, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		k := strings.ToLower(p)
		if normKeywords[k] || k == "test" || k == "tests" || k == "spec" {
			parts[i] = k
		} else {
			parts[i] = "@"
		}
	}
	return strings.Join(parts, "_")
}

// looseText: value literals and digit runs -> #, identifiers -> @ except
// structural keywords (return false and return undefined are the same
// case; except: return and except: raise are not).
func looseText(s string) string {
	s = looseLit.ReplaceAllString(s, "#")
	s = looseIdent.ReplaceAllStringFunc(s, looseTok)
	s = looseDigit.ReplaceAllString(s, "#")
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = looseWS.ReplaceAllString(ln, " ")
		out = append(out, strings.TrimSpace(ln))
	}
	return strings.Join(out, "\n")
}

// loosePath: each segment's identifiers become @ (test markers stay),
// digits become #, the extension is preserved.
func loosePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		ext := ""
		if j := strings.LastIndex(s, "."); j >= 0 {
			ext = s[j:]
			s = s[:j]
		}
		s = looseLit.ReplaceAllString(s, "#")
		s = looseIdent.ReplaceAllStringFunc(s, looseTok)
		segs[i] = looseDigit.ReplaceAllString(s, "#") + ext
	}
	return strings.Join(segs, "/")
}
