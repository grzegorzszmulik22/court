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
		if positive {
			pos++
			for _, h := range vec.Expect {
				rulesUsed[h.Rule] = true
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
		if h.Detector != vec.Detector {
			v.addf("vector-hit", hw, "hit detector %q != case detector %q", h.Detector, vec.Detector)
		}
		if detectorCategory[vec.Detector] != h.Category {
			v.addf("vector-hit", hw, "category %q is not %s's category", h.Category, vec.Detector)
		}
		if subs := detectorSubkinds[vec.Detector]; len(subs) == 0 {
			if h.Subkind != "" {
				v.addf("vector-hit", hw, "%s has no subkinds; hit must leave subkind empty", vec.Detector)
			}
		} else if !contains(subs, h.Subkind) {
			v.addf("vector-hit", hw, "subkind %q not in %s's subkind list", h.Subkind, vec.Detector)
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
		default:
			v.addf("vector-hit", hw, "side %q unknown", h.Side)
		}
	}
	_ = rules
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
