package ledger

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	apischema "court/api/schema"
)

// This file implements the small JSON-Schema subset used by api/schema/*.json:
// type, enum, const, required, properties, additionalProperties (bool),
// pattern, minLength, maxLength, minimum, maximum, items, minItems, maxItems,
// uniqueItems, anyOf, allOf, if/then/else, $defs and local $ref.
// refledger (Python) implements the same subset independently.

// SchemaError describes one instance/schema mismatch.
type SchemaError struct{ Path, Msg string }

func (e *SchemaError) Error() string { return e.Path + ": " + e.Msg }

// Schemas is the set of loaded event schemas, keyed by event type.
var Schemas = map[string]*Value{}

// LoadSchemas parses every *.schema.json document in dir into Schemas.
// dir may be a filesystem path or "-" to load the embedded set.
func LoadSchemas(dir string) error {
	Schemas = map[string]*Value{}
	for _, t := range TypeNames() {
		raw, err := schemaBytes(dir, t)
		if err != nil {
			return err
		}
		v, err := ParseStrict(raw)
		if err != nil {
			return fmt.Errorf("schema %s: %w", t, err)
		}
		Schemas[t] = v
	}
	return nil
}

func schemaBytes(dir, typ string) ([]byte, error) {
	name := typ + ".schema.json"
	if dir == "" || dir == "-" {
		return apischema.FS.ReadFile(name)
	}
	return os.ReadFile(filepath.Join(dir, name))
}

// ValidateSchema checks instance against a parsed schema document, collecting
// all mismatches. Returns nil when valid.
func ValidateSchema(sch *Value, inst *Value) []*SchemaError {
	v := &schemaCheck{root: sch}
	v.check(inst, sch, "$", "$")
	return v.errs
}

type schemaCheck struct {
	root *Value
	errs []*SchemaError
	refs int
}

func (c *schemaCheck) fail(path, msg string) {
	c.errs = append(c.errs, &SchemaError{path, msg})
}

func objGet(v *Value, key string) (*Value, bool) {
	if v == nil || v.Kind != Object {
		return nil, false
	}
	for _, m := range v.Obj {
		if m.Key == key {
			return m.Val, true
		}
	}
	return nil, false
}

func strOf(v *Value) string {
	if v == nil || v.Kind != String {
		return ""
	}
	return v.Str
}

func numOf(v *Value) (float64, bool) {
	if v == nil || v.Kind != Number {
		return 0, false
	}
	f, err := strconv.ParseFloat(v.Num, 64)
	return f, err == nil
}

func boolOf(v *Value) bool { return v != nil && v.Kind == Bool && v.Bool }

func (c *schemaCheck) check(inst, sch *Value, path, spath string) {
	if sch == nil || sch.Kind != Object {
		c.fail(path, "schema at "+spath+" is not an object")
		return
	}
	if r, ok := objGet(sch, "$ref"); ok {
		if c.refs > 64 {
			c.fail(path, "too many $ref hops")
			return
		}
		c.refs++
		target, err := c.resolve(strOf(r))
		if err != nil {
			c.fail(path, err.Error())
			return
		}
		c.check(inst, target, path, strOf(r))
		c.refs--
		return
	}
	if a, ok := objGet(sch, "allOf"); ok && a.Kind == Array {
		for i, sub := range a.Arr {
			c.check(inst, sub, path, spath+".allOf["+strconv.Itoa(i)+"]")
		}
	}
	if a, ok := objGet(sch, "anyOf"); ok && a.Kind == Array {
		inner := &schemaCheck{root: c.root}
		ok1 := false
		for _, sub := range a.Arr {
			inner.errs = nil
			inner.check(inst, sub, path, spath+".anyOf")
			if len(inner.errs) == 0 {
				ok1 = true
				break
			}
		}
		if !ok1 {
			c.fail(path, "no anyOf branch matches")
		}
	}
	if n, ok := objGet(sch, "not"); ok {
		inner := &schemaCheck{root: c.root}
		inner.check(inst, n, path, spath+".not")
		if len(inner.errs) == 0 {
			c.fail(path, "matches 'not' schema")
		}
	}
	if e, ok := objGet(sch, "enum"); ok && e.Kind == Array {
		found := false
		for _, ev := range e.Arr {
			if equalJSON(inst, ev) {
				found = true
				break
			}
		}
		if !found {
			c.fail(path, "not in enum")
		}
	}
	if k, ok := objGet(sch, "const"); ok && !equalJSON(inst, k) {
		c.fail(path, "does not equal const")
	}
	if t, ok := objGet(sch, "type"); ok {
		c.checkType(inst, strOf(t), path)
	}
	if inst == nil {
		return
	}
	// ---- object keywords ----
	if inst.Kind == Object {
		props, _ := objGet(sch, "properties")
		if req, ok := objGet(sch, "required"); ok && req.Kind == Array {
			for _, r := range req.Arr {
				if _, ok := objGet(inst, strOf(r)); !ok {
					c.fail(path, "missing required property "+strOf(r))
				}
			}
		}
		ap, hasAP := objGet(sch, "additionalProperties")
		apOff := hasAP && ap.Kind == Bool && !ap.Bool
		for _, m := range inst.Obj {
			var sub *Value
			if props != nil && props.Kind == Object {
				if p, ok := objGet(props, m.Key); ok {
					sub = p
				}
			}
			if sub == nil {
				if apOff {
					c.fail(path, "additional property "+strconv.Quote(m.Key))
				} else if hasAP && ap.Kind == Object {
					c.check(m.Val, ap, path+"."+m.Key, spath+".additionalProperties")
				}
				continue
			}
			c.check(m.Val, sub, path+"."+m.Key, spath+".properties."+m.Key)
		}
	}
	// ---- array keywords ----
	if inst.Kind == Array {
		if it, ok := objGet(sch, "items"); ok {
			for i, e := range inst.Arr {
				c.check(e, it, path+"["+strconv.Itoa(i)+"]", spath+".items")
			}
		}
		if n, ok := objGet(sch, "minItems"); ok {
			if f, ok := numOf(n); ok && float64(len(inst.Arr)) < f {
				c.fail(path, "fewer than minItems")
			}
		}
		if n, ok := objGet(sch, "maxItems"); ok {
			if f, ok := numOf(n); ok && float64(len(inst.Arr)) > f {
				c.fail(path, "more than maxItems")
			}
		}
		if boolOf(mustGet(sch, "uniqueItems")) {
			seen := map[string]bool{}
			for _, e := range inst.Arr {
				k := string(MustCanonical(e))
				if seen[k] {
					c.fail(path, "duplicate array element")
					break
				}
				seen[k] = true
			}
		}
	}
	// ---- string keywords ----
	if inst.Kind == String {
		rs := []rune(inst.Str)
		if n, ok := objGet(sch, "minLength"); ok {
			if f, ok := numOf(n); ok && float64(len(rs)) < f {
				c.fail(path, "shorter than minLength")
			}
		}
		if n, ok := objGet(sch, "maxLength"); ok {
			if f, ok := numOf(n); ok && float64(len(rs)) > f {
				c.fail(path, "longer than maxLength")
			}
		}
		if p, ok := objGet(sch, "pattern"); ok {
			re, err := regexp.Compile(strOf(p))
			if err != nil || !re.MatchString(inst.Str) {
				c.fail(path, "does not match pattern")
			}
		}
	}
	// ---- number keywords ----
	if inst.Kind == Number {
		f, _ := numOf(inst)
		if n, ok := objGet(sch, "minimum"); ok {
			if m, ok := numOf(n); ok && f < m {
				c.fail(path, "below minimum")
			}
		}
		if n, ok := objGet(sch, "maximum"); ok {
			if m, ok := numOf(n); ok && f > m {
				c.fail(path, "above maximum")
			}
		}
	}
	// ---- conditional ----
	if iv, ok := objGet(sch, "if"); ok {
		inner := &schemaCheck{root: c.root}
		inner.check(inst, iv, path, spath+".if")
		if len(inner.errs) == 0 {
			if th, ok := objGet(sch, "then"); ok {
				c.check(inst, th, path, spath+".then")
			}
		} else if el, ok := objGet(sch, "else"); ok {
			c.check(inst, el, path, spath+".else")
		}
	}
}

func mustGet(v *Value, key string) *Value {
	x, _ := objGet(v, key)
	return x
}

func (c *schemaCheck) checkType(inst *Value, t, path string) {
	if inst == nil {
		return
	}
	ok := false
	switch t {
	case "object":
		ok = inst.Kind == Object
	case "array":
		ok = inst.Kind == Array
	case "string":
		ok = inst.Kind == String
	case "boolean":
		ok = inst.Kind == Bool
	case "null":
		ok = inst.Kind == Null
	case "integer":
		ok = inst.Kind == Number && isIntegral(inst.Num)
	case "number":
		ok = inst.Kind == Number
	}
	if !ok {
		c.fail(path, "type is not "+t)
	}
}

func isIntegral(lit string) bool {
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	return math.Abs(f) < 9007199254740992 && f == math.Trunc(f)
}

func (c *schemaCheck) resolve(ref string) (*Value, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported $ref %q", ref)
	}
	cur := c.root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		next, ok := objGet(cur, part)
		if !ok {
			return nil, fmt.Errorf("$ref %q not found", ref)
		}
		cur = next
	}
	return cur, nil
}

// equalJSON compares two parsed values recursively (canonical form).
func equalJSON(a, b *Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Null:
		return true
	case Bool:
		return a.Bool == b.Bool
	case Number:
		fa, ea := strconv.ParseFloat(a.Num, 64)
		fb, eb := strconv.ParseFloat(b.Num, 64)
		return ea == nil && eb == nil && fa == fb
	case String:
		return a.Str == b.Str
	case Array:
		if len(a.Arr) != len(b.Arr) {
			return false
		}
		for i := range a.Arr {
			if !equalJSON(a.Arr[i], b.Arr[i]) {
				return false
			}
		}
		return true
	case Object:
		return string(MustCanonical(a)) == string(MustCanonical(b))
	}
	return false
}
