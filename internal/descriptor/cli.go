package descriptor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CLI is an op's ergonomic surface — the per-op block docs/CLI-DESIGN.md §4 describes,
// from which the `safe_cli <area> <verb> --flags` tree is generated. It comes in one of
// three shapes, exactly one of which must be set:
//
//   - a verb: Area+Verb (+Priority, Target, Summary, the flags and templates below);
//   - AliasOf "entity.op": this op hits the same route as another and generates nothing;
//   - CallOnly with a Reason: deliberately left on the generic `call` (device-originated
//     telemetry, SDK plumbing, child-side asks).
//
// The block is validated at Parse so a wrong request is impossible to generate: every
// body-template variable is a flag or a resolved value, every retained literal field is a
// declared constant (an unclassified body_example field is rejected — examples carry
// request-specific sample data), and every query name the op declares has one source.
type CLI struct {
	Area     string `json:"area,omitempty"`
	Verb     string `json:"verb,omitempty"`
	Group    string `json:"group,omitempty"`    // optional third level (`device settings show`)
	Priority string `json:"priority,omitempty"` // core | common | long-tail
	Target   string `json:"target,omitempty"`   // account | self | child | device
	// Summary is the one-line help shown in `<area> --help`; core verbs are listed first.
	Summary string `json:"summary,omitempty"`
	// Prereq lines render as "Prerequisite:" in the verb's help (e.g. the phone must be PAIRED).
	Prereq []string `json:"prereq,omitempty"`
	Auth   string   `json:"auth,omitempty"` // id_token (default) | spc_token
	// BodyTemplate is the op's body_example with variables. It is strict JSON: a variable is
	// a JSON string "$name" (or "$name?" — the property is omitted entirely when the variable
	// is unset or nulled by another flag's Nulls), and the engine substitutes the typed value.
	// Every leaf is either a variable or a field declared in Constants.
	BodyTemplate string `json:"body_template,omitempty"`
	Flags        []Flag `json:"flags,omitempty"`
	// Query maps a declared query-parameter name to a constant value or to "$flag" — the
	// descriptor's name-only `query` list cannot carry a value, so every constant a
	// zero-flag verb needs (categorySupported=v6, strategy=NotNull) is declared here.
	Query map[string]string `json:"query,omitempty"`
	// Constants declares, by name, the body fields kept baked from the example
	// (MAPPVersion, editSource, productType). A template literal not listed here is rejected.
	Constants map[string]any `json:"constants,omitempty"`
	// Resolve lists the variables the engine fills itself: $child.serviceId|profileId|
	// deviceId|pairing, $self.serviceId|profileId, $account.id|timezone, $now.epochMs, $uuid,
	// and $lookup:<entity>.<op>:<key>=<flag>:<field> for enrichment reads.
	Resolve []string `json:"resolve,omitempty"`
	// Variants picks the op by whether --child is given ({"account": "x.y", "child": "x.z"}).
	Variants      map[string]string `json:"variants,omitempty"`
	AliasOf       string            `json:"alias_of,omitempty"`
	CallOnly      bool              `json:"call_only,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	LiveEmergency bool              `json:"live_emergency,omitempty"` // requires --confirm and warns
	Output        *Output           `json:"output,omitempty"`
}

// Flag is one typed --flag of a generated verb and where its value goes.
type Flag struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"` // string|int|float|bool|enum|duration|date|datetime|tz|list
	Enum       []string `json:"enum,omitempty"`
	Default    any      `json:"default,omitempty"`
	Required   bool     `json:"required,omitempty"`
	Repeatable bool     `json:"repeatable,omitempty"`
	Excludes   []string `json:"excludes,omitempty"` // flags that may not be combined with this one
	Nulls      []string `json:"nulls,omitempty"`    // flags whose variable this one unsets (their "$x?" property is omitted)
	// MapsTo is body:$var | query:<name> | header:<name> | path:<placeholder>.
	MapsTo    string `json:"maps_to"`
	Transform string `json:"transform,omitempty"`
	Help      string `json:"help"`
}

// Output names the response fields the default table shows.
type Output struct {
	Table []string `json:"table,omitempty"`
}

// The closed vocabularies the generator and engine understand. A name outside these is a
// descriptor bug caught at load, not a silently-ignored field.
var (
	cliPriorities = set("core", "common", "long-tail")
	cliTargets    = set("account", "self", "child", "device")
	cliAuths      = set("", "id_token", "spc_token")
	cliFlagTypes  = set("string", "int", "float", "bool", "enum", "duration", "date", "datetime", "tz", "list")
	// cliTransforms is the engine's fixed registry (docs/CLI-DESIGN.md §4); the engine is the
	// only place that implements them, this list only rejects an unknown name early.
	cliTransforms = set("", "pause_schedule", "tz_short", "iso_micro", "epoch_ms", "day3_lower", "day3_title", "weekday_ints", "bool01", "allow_block_ab")
	// cliResolvePrefixes are the resolved-variable families the engine can fill.
	cliResolvePrefixes = []string{"$child.", "$self.", "$account.", "$now.", "$uuid", "$lookup:"}
)

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// validateCLIBlocks checks every op's `cli` block (invariants 2–4 of the architecture
// pass). lookup resolves "entity.op" for alias_of targets.
func (d *Descriptor) validateCLIBlocks() error {
	for _, ename := range d.EntityNames() {
		e := d.Entities[ename]
		check := func(ops map[string]Operation) error {
			names := make([]string, 0, len(ops))
			for k := range ops {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, oname := range names {
				o := ops[oname]
				if o.CLI == nil {
					continue
				}
				if err := d.validateCLI(o); err != nil {
					return fmt.Errorf("%s.%s cli: %w", ename, oname, err)
				}
			}
			return nil
		}
		if err := check(e.Operations); err != nil {
			return err
		}
		if err := check(e.Actions); err != nil {
			return err
		}
	}
	return nil
}

// opExists reports whether "entity.op" names an operation or action.
func (d *Descriptor) opExists(ref string) bool {
	ent, op, ok := strings.Cut(ref, ".")
	if !ok {
		return false
	}
	e, ok := d.Entities[ent]
	if !ok {
		return false
	}
	if _, ok := e.Operations[op]; ok {
		return true
	}
	_, ok = e.Actions[op]
	return ok
}

func (d *Descriptor) validateCLI(o Operation) error {
	c := o.CLI
	// Exactly one shape.
	isVerb := c.Area != "" || c.Verb != ""
	shapes := 0
	for _, on := range []bool{isVerb, c.AliasOf != "", c.CallOnly} {
		if on {
			shapes++
		}
	}
	if shapes != 1 {
		return fmt.Errorf("must be exactly one of a verb (area+verb), alias_of, or call_only")
	}
	if c.AliasOf != "" {
		if !d.opExists(c.AliasOf) {
			return fmt.Errorf("alias_of %q does not name an existing entity.op", c.AliasOf)
		}
		return nil
	}
	if c.CallOnly {
		if strings.TrimSpace(c.Reason) == "" {
			return fmt.Errorf("call_only needs a reason")
		}
		return nil
	}
	// A verb.
	if c.Area == "" || c.Verb == "" {
		return fmt.Errorf("a verb needs both area and verb")
	}
	if !cliPriorities[c.Priority] {
		return fmt.Errorf("priority %q must be core|common|long-tail", c.Priority)
	}
	if !cliTargets[c.Target] {
		return fmt.Errorf("target %q must be account|self|child|device", c.Target)
	}
	if strings.TrimSpace(c.Summary) == "" {
		return fmt.Errorf("a verb needs a summary (its --help line)")
	}
	if !cliAuths[c.Auth] {
		return fmt.Errorf("auth %q must be id_token|spc_token", c.Auth)
	}
	for k, ref := range c.Variants {
		if !d.opExists(ref) {
			return fmt.Errorf("variants[%s] %q does not name an existing entity.op", k, ref)
		}
	}

	// Flags: unique names, known types, well-formed maps_to.
	flagByName := make(map[string]Flag, len(c.Flags))
	bodyVarByFlag := make(map[string]Flag) // body var name -> flag
	queryFromFlag := make(map[string]bool) // query name -> covered by a flag
	for _, f := range c.Flags {
		if f.Name == "" {
			return fmt.Errorf("a flag has no name")
		}
		if _, dup := flagByName[f.Name]; dup {
			return fmt.Errorf("flag --%s declared twice", f.Name)
		}
		flagByName[f.Name] = f
		if !cliFlagTypes[f.Type] {
			return fmt.Errorf("flag --%s: type %q is not a known flag type", f.Name, f.Type)
		}
		if f.Type == "enum" && len(f.Enum) == 0 {
			return fmt.Errorf("flag --%s: type enum needs an enum list", f.Name)
		}
		if !cliTransforms[f.Transform] {
			return fmt.Errorf("flag --%s: transform %q is not in the engine's registry", f.Name, f.Transform)
		}
		if strings.TrimSpace(f.Help) == "" {
			return fmt.Errorf("flag --%s: needs help text (every flag's help states its default and effect)", f.Name)
		}
		kind, arg, ok := strings.Cut(f.MapsTo, ":")
		if !ok || arg == "" {
			return fmt.Errorf("flag --%s: maps_to %q must be body:$var | query:<name> | header:<name> | path:<placeholder>", f.Name, f.MapsTo)
		}
		switch kind {
		case "body":
			if !strings.HasPrefix(arg, "$") {
				return fmt.Errorf("flag --%s: body maps_to %q must be a $var", f.Name, f.MapsTo)
			}
			bodyVarByFlag[strings.TrimPrefix(arg, "$")] = f
		case "query":
			if !contains(o.Query, arg) {
				return fmt.Errorf("flag --%s: query %q is not one of the op's declared query params %v", f.Name, arg, o.Query)
			}
			queryFromFlag[arg] = true
		case "header", "path":
			// header names and path placeholders are free-form here; fillPath/headers check at call time.
		default:
			return fmt.Errorf("flag --%s: maps_to kind %q must be body|query|header|path", f.Name, kind)
		}
	}
	for _, f := range c.Flags {
		for _, x := range append(append([]string{}, f.Excludes...), f.Nulls...) {
			if _, ok := flagByName[x]; !ok {
				return fmt.Errorf("flag --%s: excludes/nulls names unknown flag %q", f.Name, x)
			}
		}
	}

	// Query map: every key is a declared query name; "$x" values name a flag; each required
	// query param has exactly one source.
	for name, v := range c.Query {
		if !contains(o.Query, name) {
			return fmt.Errorf("query[%s] is not one of the op's declared query params %v", name, o.Query)
		}
		if queryFromFlag[name] {
			return fmt.Errorf("query[%s] is declared as a constant and also mapped from a flag", name)
		}
		if strings.HasPrefix(v, "$") {
			if _, ok := flagByName[strings.TrimPrefix(v, "$")]; !ok {
				return fmt.Errorf("query[%s] references unknown flag %q", name, v)
			}
		}
	}
	for _, req := range o.RequiredQuery {
		if _, ok := c.Query[req]; !ok && !queryFromFlag[req] {
			return fmt.Errorf("required query param %q has no source (no flag maps to it and it is not a query constant)", req)
		}
	}

	// Body template: strict JSON; every leaf is a variable or a declared constant.
	if c.BodyTemplate == "" {
		if o.TakesBody && !o.Multipart {
			return fmt.Errorf("op takes a body but the verb has no body_template")
		}
		if len(bodyVarByFlag) > 0 {
			return fmt.Errorf("flags map to body vars but there is no body_template")
		}
		return nil
	}
	var tpl any
	if err := json.Unmarshal([]byte(c.BodyTemplate), &tpl); err != nil {
		return fmt.Errorf("body_template is not strict JSON (write variables as \"$name\" strings): %w", err)
	}
	seenVars := make(map[string]bool)
	if err := walkTemplate("", tpl, c, bodyVarByFlag, seenVars); err != nil {
		return err
	}
	for v, f := range bodyVarByFlag {
		if !seenVars[v] {
			return fmt.Errorf("flag --%s maps to body var $%s, which the body_template never uses", f.Name, v)
		}
	}
	for _, r := range c.Resolve {
		if !isResolveVar(r) {
			return fmt.Errorf("resolve entry %q is not a known resolved-variable family", r)
		}
	}
	return nil
}

// walkTemplate classifies every leaf of the template: "$name"/"$name?" must be a flag's body
// var or a resolve entry (and "?" only on a flag that can be unset); any other scalar, or
// array of scalars, must sit under a key declared in Constants.
func walkTemplate(key string, v any, c *CLI, bodyVarByFlag map[string]Flag, seen map[string]bool) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := walkTemplate(k, t[k], c, bodyVarByFlag, seen); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, el := range t {
			if _, isObj := el.(map[string]any); isObj {
				if err := walkTemplate(key, el, c, bodyVarByFlag, seen); err != nil {
					return err
				}
				continue
			}
			if s, ok := el.(string); ok && strings.HasPrefix(s, "$") {
				if err := classifyVar(s, c, bodyVarByFlag, seen); err != nil {
					return err
				}
				continue
			}
			if _, declared := c.Constants[key]; !declared {
				return fmt.Errorf("body_template field %q is an unclassified example value (array) — map it to a flag, resolve it, or declare it in constants", key)
			}
		}
		return nil
	case string:
		if strings.HasPrefix(t, "$") {
			return classifyVar(t, c, bodyVarByFlag, seen)
		}
	}
	// A literal scalar leaf.
	if _, declared := c.Constants[key]; !declared {
		return fmt.Errorf("body_template field %q is an unclassified example value — map it to a flag, resolve it, or declare it in constants", key)
	}
	return nil
}

func classifyVar(s string, c *CLI, bodyVarByFlag map[string]Flag, seen map[string]bool) error {
	optional := strings.HasSuffix(s, "?")
	name := strings.TrimSuffix(strings.TrimPrefix(s, "$"), "?")
	if f, ok := bodyVarByFlag[name]; ok {
		seen[name] = true
		if optional && f.Required && !nulledBySomeFlag(f.Name, c) {
			return fmt.Errorf("body var $%s? is optional but flag --%s is required and nothing nulls it", name, f.Name)
		}
		return nil
	}
	if isResolveVar("$" + name) {
		if optional {
			return fmt.Errorf("resolved var $%s cannot be optional (\"?\")", name)
		}
		if !contains(c.Resolve, "$"+name) {
			return fmt.Errorf("body var $%s is a resolved value but is not listed in resolve", name)
		}
		return nil
	}
	return fmt.Errorf("body var $%s is neither a flag's body var nor a resolved value", name)
}

func nulledBySomeFlag(flag string, c *CLI) bool {
	for _, f := range c.Flags {
		if contains(f.Nulls, flag) {
			return true
		}
	}
	return false
}

func isResolveVar(v string) bool {
	for _, p := range cliResolvePrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
