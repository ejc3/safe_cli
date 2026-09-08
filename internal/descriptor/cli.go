package descriptor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
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
	// Headers maps a header the op declares to the fixed value this verb sends when no flag
	// supplies it (`account set` -> x-pending-activation=false), analogous to Query. Headers
	// the op already fixes in header_values are auto-sent; identity/trace headers are filled
	// by the engine.
	Headers map[string]string `json:"headers,omitempty"`
	// Constants declares, by name, the body fields kept baked from the example
	// (MAPPVersion, editSource, productType). A template literal not listed here is rejected.
	Constants map[string]any `json:"constants,omitempty"`
	// Resolve lists the variables the engine fills itself: $child.serviceId|profileId|
	// deviceId|pairing, $self.serviceId|profileId, $account.id, $local.timezone, $now.epochMs, $uuid,
	// and $lookup:<entity>.<op>:<key>=<flag>:<field> for enrichment reads.
	Resolve []string `json:"resolve,omitempty"`
	// Select picks the operation by how the verb is invoked: an ordered list of {when, op};
	// the first matching condition wins and the block's own op is the default. Conditions:
	// "child" (--child was given), "flag:<name>" (the flag was explicitly given — `calls log
	// --number`), "exists:$lookup:..." (a lookup found a record — `screen-time set` PUT vs POST).
	Select []SelectRule `json:"select,omitempty"`
	// AtLeastOne requires at least one of the named optional flags (`account set
	// [--family-name] [--timezone]`); validated to name declared, non-required flags.
	AtLeastOne []string `json:"at_least_one,omitempty"`
	// OneOf requires exactly one alternative group of optional flags to be fully given
	// (`pick-me-up request --lat --lon | --address`); every name must be a declared flag.
	OneOf         [][]string `json:"one_of,omitempty"`
	AliasOf       string     `json:"alias_of,omitempty"`
	CallOnly      bool       `json:"call_only,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	LiveEmergency bool       `json:"live_emergency,omitempty"` // requires --confirm and warns
	Output        *Output    `json:"output,omitempty"`
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
	Requires   []string `json:"requires,omitempty"` // flags that must be given whenever this one is (--lat requires --lon)
	// MapsTo is body:$var | query:<name> | header:<name> | path:<placeholder>. A flag has
	// exactly one of MapsTo or SpreadsTo.
	MapsTo string `json:"maps_to,omitempty"`
	// SpreadsTo lets one semantic flag populate several body vars through a STRUCTURED
	// transform that returns one value per destination (`--mode block|alert` ->
	// blockContent + alertOn via mode_block_alert, a wire-verified exclusive pair).
	SpreadsTo []string `json:"spreads_to,omitempty"`
	Transform string   `json:"transform,omitempty"`
	Help      string   `json:"help"`
}

// SelectRule is one {when, op} entry of CLI.Select. A branch may override the verb's
// target, body_template, query, constants and resolve — unspecified fields inherit — because
// a branch can be a different request contract (`alerts settings set --child` posts
// different keys under a different header). Each branch is validated as a complete verb
// against its own op.
type SelectRule struct {
	When         string            `json:"when"`
	Op           string            `json:"op"`
	Target       string            `json:"target,omitempty"`
	BodyTemplate string            `json:"body_template,omitempty"`
	Query        map[string]string `json:"query,omitempty"`
	Constants    map[string]any    `json:"constants,omitempty"`
	Resolve      []string          `json:"resolve,omitempty"`
}

// UnmarshalJSON rejects unknown keys: the block's fields are optional, so a misspelled
// key would otherwise be silently discarded and the block would still validate —
// generating an untransformed body or dropping a guard. Flag, SelectRule and CLIBlocks
// decode the same way.
func (c *CLI) UnmarshalJSON(b []byte) error {
	type plain CLI
	var p plain
	if err := strictDecode(b, &p); err != nil {
		return fmt.Errorf("cli block: %w", err)
	}
	*c = CLI(p)
	return nil
}

// UnmarshalJSON rejects unknown keys (see CLI.UnmarshalJSON).
func (f *Flag) UnmarshalJSON(b []byte) error {
	type plain Flag
	var p plain
	if err := strictDecode(b, &p); err != nil {
		return fmt.Errorf("flag: %w", err)
	}
	*f = Flag(p)
	return nil
}

// UnmarshalJSON rejects unknown keys (see CLI.UnmarshalJSON).
func (r *SelectRule) UnmarshalJSON(b []byte) error {
	type plain SelectRule
	var p plain
	if err := strictDecode(b, &p); err != nil {
		return fmt.Errorf("select: %w", err)
	}
	*r = SelectRule(p)
	return nil
}

func strictDecode(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Output names the response fields the default table shows.
type Output struct {
	Table []string `json:"table,omitempty"`
}

// CLIBlocks is an op's `cli` entry: one verb block, or a list of them when one operation
// backs several verbs (content_filter.updateSubcategory drives both `filter block` and
// `filter allow`, differing by a constant). It decodes from a JSON object or array; an
// alias_of or call_only entry must be the only one.
type CLIBlocks []*CLI

// UnmarshalJSON accepts a single verb block or a list of them.
func (b *CLIBlocks) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var list []*CLI
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		*b = list
		return nil
	}
	var one CLI
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return err
	}
	*b = CLIBlocks{&one}
	return nil
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
	// weekday_ints (postScheduleAlert's weekDays) is absent on purpose: every captured
	// example has weekDays: [] so its int convention is unobserved; it joins when grounded.
	cliTransforms = set("", "pause_schedule", "tz_short", "iso_micro", "epoch_ms", "day3_lower", "day3_title", "bool01", "allow_block_ab")
	// cliStructuredTransforms fill a FIXED set of body vars each; a flag's spreads_to must
	// name exactly that set (mode_block_alert -> blockContent + alertOn, the wire-verified
	// exclusive pair), so the engine's returned keys always have a destination.
	cliStructuredTransforms = map[string][]string{
		"mode_block_alert": {"blockContent", "alertOn"},
	}
	// cliResolveNames is the EXACT vocabulary of resolved variables the engine can fill
	// (plus the structured $lookup form checked by checkResolveVar). Exact, not a prefix:
	// a typo like $child.profielId must fail at load, not reach the engine.
	cliResolveNames = set("$child.serviceId", "$child.profileId", "$child.deviceId", "$child.pairing",
		"$self.serviceId", "$self.profileId", "$account.id", "$local.timezone", "$now.epochMs", "$uuid")
	// cliResolveFamilies only decide which error a bad "$x" gets (a mistyped resolved
	// variable vs. something that is not a resolved variable at all).
	cliResolveFamilies = []string{"$child.", "$self.", "$account.", "$local.", "$now.", "$uuid", "$lookup:"}
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
				seenVerb := make(map[string]bool, len(o.CLI))
				for i, c := range o.CLI {
					if len(o.CLI) > 1 && (c.AliasOf != "" || c.CallOnly) {
						return fmt.Errorf("%s.%s cli[%d]: alias_of or call_only must be the only entry of an op's cli list", ename, oname, i)
					}
					if c.Verb != "" {
						key := c.Area + " " + c.Verb
						if seenVerb[key] {
							return fmt.Errorf("%s.%s cli[%d]: verb %s declared twice on this op", ename, oname, i, key)
						}
						seenVerb[key] = true
					}
					if err := d.validateCLI(o, c); err != nil {
						return fmt.Errorf("%s.%s cli[%d]: %w", ename, oname, i, err)
					}
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

func (d *Descriptor) validateCLI(o Operation, c *CLI) error {
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
	if strings.TrimSpace(c.Summary) == "" {
		return fmt.Errorf("a verb needs a summary (its --help line)")
	}
	if !cliAuths[c.Auth] {
		return fmt.Errorf("auth %q must be id_token|spc_token", c.Auth)
	}
	if err := d.validateContract(o, c); err != nil {
		return err
	}
	// select: conditions are checked against the verb's flags, and each branch is a
	// complete request contract — the verb's fields with the branch's overrides — validated
	// against ITS op, so choosing an op can never apply the base template to an unrelated
	// route.
	flagByName := flagIndex(c.Flags)
	for i, r := range c.Select {
		switch {
		case r.When == "child":
		case strings.HasPrefix(r.When, "flag:"):
			if _, ok := flagByName[strings.TrimPrefix(r.When, "flag:")]; !ok {
				return fmt.Errorf("select[%d]: condition %q names unknown flag %q", i, r.When, strings.TrimPrefix(r.When, "flag:"))
			}
		case strings.HasPrefix(r.When, "exists:"):
			if err := d.checkResolveVar(strings.TrimPrefix(r.When, "exists:"), flagByName); err != nil {
				return fmt.Errorf("select[%d]: %w", i, err)
			}
		default:
			return fmt.Errorf("select[%d]: condition %q must be child | flag:<name> | exists:$lookup:<entity>.<op>:<key>=<flag>:<field>", i, r.When)
		}
		bo, ok := d.lookupOp(r.Op)
		if !ok {
			return fmt.Errorf("select[%d]: op %q does not name an existing entity.op", i, r.Op)
		}
		if err := d.validateContract(bo, c.withOverrides(r)); err != nil {
			return fmt.Errorf("select[%d] (%s): %w", i, r.Op, err)
		}
	}
	return nil
}

// lookupOp returns the operation "entity.op" names.
func (d *Descriptor) lookupOp(ref string) (Operation, bool) {
	ent, op, ok := strings.Cut(ref, ".")
	if !ok {
		return Operation{}, false
	}
	e, ok := d.Entities[ent]
	if !ok {
		return Operation{}, false
	}
	if o, ok := e.Operations[op]; ok {
		return o, true
	}
	o, ok := e.Actions[op]
	return o, ok
}

// withOverrides returns a select branch's request contract: a copy of the verb with the
// branch's non-empty overrides applied and no further select.
func (c *CLI) withOverrides(r SelectRule) *CLI {
	m := *c
	m.Select = nil
	if r.Target != "" {
		m.Target = r.Target
	}
	if r.BodyTemplate != "" {
		m.BodyTemplate = r.BodyTemplate
	}
	if r.Query != nil {
		m.Query = r.Query
	}
	if r.Constants != nil {
		m.Constants = r.Constants
	}
	if r.Resolve != nil {
		m.Resolve = r.Resolve
	}
	return &m
}

func flagIndex(flags []Flag) map[string]Flag {
	m := make(map[string]Flag, len(flags))
	for _, f := range flags {
		m[f.Name] = f
	}
	return m
}

// validateContract checks a verb's request contract — target, flags and their
// destinations, dependent-flag groups, query sources, resolve entries and the body
// template — against op o (invariants 2–4 of the architecture pass).
func (d *Descriptor) validateContract(o Operation, c *CLI) error {
	if !cliTargets[c.Target] {
		return fmt.Errorf("target %q must be account|self|child|device", c.Target)
	}
	// Flags: unique names, known types, exactly one destination form, well-formed maps_to.
	flagByName := make(map[string]Flag, len(c.Flags))
	bodyVarByFlag := make(map[string]Flag)   // body var name -> flag
	queryFromFlag := make(map[string]string) // query name -> the flag that maps to it
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
		if strings.TrimSpace(f.Help) == "" {
			return fmt.Errorf("flag --%s: needs help text (every flag's help states its default and effect)", f.Name)
		}
		// Exactly one destination form: maps_to (one destination, scalar transform) or
		// spreads_to (several body vars, structured transform).
		if (f.MapsTo == "") == (len(f.SpreadsTo) == 0) {
			return fmt.Errorf("flag --%s: needs exactly one of maps_to or spreads_to", f.Name)
		}
		if len(f.SpreadsTo) > 0 {
			want, structured := cliStructuredTransforms[f.Transform]
			if !structured {
				return fmt.Errorf("flag --%s: spreads_to needs a structured transform (one of %s), got %q", f.Name, joinKeys(cliStructuredTransforms), f.Transform)
			}
			if got := spreadVars(f.SpreadsTo); !sameSet(got, want) {
				return fmt.Errorf("flag --%s: transform %s must spread to exactly %v, got %v", f.Name, f.Transform, want, got)
			}
			for _, dest := range f.SpreadsTo {
				kind, arg, ok := strings.Cut(dest, ":")
				if !ok || kind != "body" || !strings.HasPrefix(arg, "$") {
					return fmt.Errorf("flag --%s: spreads_to destination %q must be body:$var", f.Name, dest)
				}
				v := strings.TrimPrefix(arg, "$")
				if prev, dup := bodyVarByFlag[v]; dup {
					return fmt.Errorf("flag --%s: body var $%s is already mapped from --%s (two flags cannot compete for one template value)", f.Name, v, prev.Name)
				}
				bodyVarByFlag[v] = f
			}
			continue
		}
		if !cliTransforms[f.Transform] {
			return fmt.Errorf("flag --%s: transform %q is not in the engine's registry", f.Name, f.Transform)
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
			v := strings.TrimPrefix(arg, "$")
			if prev, dup := bodyVarByFlag[v]; dup {
				return fmt.Errorf("flag --%s: body var $%s is already mapped from --%s (two flags cannot compete for one template value)", f.Name, v, prev.Name)
			}
			bodyVarByFlag[v] = f
		case "query":
			if !contains(o.Query, arg) {
				return fmt.Errorf("flag --%s: query %q is not one of the op's declared query params %v", f.Name, arg, o.Query)
			}
			if prev, dup := queryFromFlag[arg]; dup {
				return fmt.Errorf("flag --%s: query %q is already mapped from --%s (one query parameter, one source)", f.Name, arg, prev)
			}
			queryFromFlag[arg] = f.Name
		case "header":
			// header names are free-form; the headers block checks them at call time.
		case "path":
			if !strings.Contains(o.Path, "{"+arg+"}") {
				return fmt.Errorf("flag --%s: path %q is not a {placeholder} in the op's path %s", f.Name, arg, o.Path)
			}
		default:
			return fmt.Errorf("flag --%s: maps_to kind %q must be body|query|header|path", f.Name, kind)
		}
	}
	for _, f := range c.Flags {
		for _, x := range f.Excludes {
			g, ok := flagByName[x]
			if !ok {
				return fmt.Errorf("flag --%s: excludes/nulls names unknown flag %q", f.Name, x)
			}
			// A defaulted flag is always populated, so "only --this" and "--this --that=<default>"
			// are indistinguishable and the exclusion cannot be decided either way. Precedence
			// over a defaulted flag is expressed with nulls (its "$var?" is omitted) instead.
			if g.Default != nil {
				return fmt.Errorf("flag --%s: excludes --%s, which has a default (a defaulted flag is always present, so the exclusion cannot be decided); express precedence with nulls instead", f.Name, x)
			}
		}
		for _, x := range f.Nulls {
			g, ok := flagByName[x]
			if !ok {
				return fmt.Errorf("flag --%s: excludes/nulls names unknown flag %q", f.Name, x)
			}
			// Omission is defined only for an optional "$x?" body property; nulling a query/
			// header/path flag or a required "$x" would leave a null or unresolved value.
			kind, arg, _ := strings.Cut(g.MapsTo, ":")
			v := strings.TrimPrefix(arg, "$")
			if kind != "body" || !strings.Contains(c.BodyTemplate, "\"$"+v+"?\"") {
				return fmt.Errorf("flag --%s: nulls --%s, whose var must be an optional body variable (\"$%s?\") in the body_template — omission is only defined for optional body properties", f.Name, x, v)
			}
		}
		for _, x := range f.Requires {
			if x == f.Name {
				return fmt.Errorf("flag --%s: requires itself", f.Name)
			}
			if _, ok := flagByName[x]; !ok {
				return fmt.Errorf("flag --%s: requires unknown flag %q", f.Name, x)
			}
		}
	}
	// at_least_one: a verb-level "one of these optional flags must be given" (account set
	// --family-name|--timezone). Names must be declared, and requiring one of a set only
	// makes sense when none of them is individually required.
	if len(c.AtLeastOne) > 0 {
		if len(c.AtLeastOne) < 2 {
			return fmt.Errorf("at_least_one needs at least two flags, got %v", c.AtLeastOne)
		}
		for _, x := range c.AtLeastOne {
			g, ok := flagByName[x]
			if !ok {
				return fmt.Errorf("at_least_one names unknown flag %q", x)
			}
			if g.Required {
				return fmt.Errorf("at_least_one names --%s, which is already required", x)
			}
		}
	}
	// one_of: exactly one alternative group must be fully given (--lat --lon | --address).
	if len(c.OneOf) > 0 {
		if len(c.OneOf) < 2 {
			return fmt.Errorf("one_of needs at least two alternative groups, got %d", len(c.OneOf))
		}
		for gi, grp := range c.OneOf {
			if len(grp) == 0 {
				return fmt.Errorf("one_of[%d] is an empty group", gi)
			}
			for _, x := range grp {
				if _, ok := flagByName[x]; !ok {
					return fmt.Errorf("one_of[%d] names unknown flag %q", gi, x)
				}
			}
		}
	}

	// Query map: every key is a declared query name; "$x" values name a flag; each required
	// query param has exactly one source.
	for name, v := range c.Query {
		if !contains(o.Query, name) {
			return fmt.Errorf("query[%s] is not one of the op's declared query params %v", name, o.Query)
		}
		if _, fromFlag := queryFromFlag[name]; fromFlag {
			return fmt.Errorf("query[%s] is declared as a constant and also mapped from a flag", name)
		}
		if strings.HasPrefix(v, "$") {
			if _, ok := flagByName[strings.TrimPrefix(v, "$")]; !ok {
				return fmt.Errorf("query[%s] references unknown flag %q", name, v)
			}
		}
	}
	// Fixed header constants: every name must be a header the op declares.
	for name := range c.Headers {
		if !contains(o.Headers, name) {
			return fmt.Errorf("headers[%s] is not one of the op's declared headers %v", name, o.Headers)
		}
	}
	for _, req := range o.RequiredQuery {
		_, fromFlag := queryFromFlag[req]
		if _, ok := c.Query[req]; !ok && !fromFlag {
			return fmt.Errorf("required query param %q has no source (no flag maps to it and it is not a query constant)", req)
		}
	}

	// Resolve entries are checked for every verb, bodyless ones included — a path, query or
	// header can resolve too, and a typo must fail here, not at invocation.
	resolveOK := func(v string) error { return d.checkResolveVar(v, flagByName) }
	for _, r := range c.Resolve {
		if err := resolveOK(r); err != nil {
			return fmt.Errorf("resolve entry: %w", err)
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
	if !o.TakesBody {
		return fmt.Errorf("op declares no body (takes_body=false) but the verb has a body_template")
	}
	var tpl any
	if err := json.Unmarshal([]byte(c.BodyTemplate), &tpl); err != nil {
		return fmt.Errorf("body_template is not strict JSON (write variables as \"$name\" strings): %w", err)
	}
	seenVars := make(map[string]bool)
	if err := walkTemplate("", tpl, c, bodyVarByFlag, seenVars, resolveOK, false, nil); err != nil {
		return err
	}
	for v, f := range bodyVarByFlag {
		if !seenVars[v] {
			return fmt.Errorf("flag --%s maps to body var $%s, which the body_template never uses", f.Name, v)
		}
	}
	return nil
}

// checkResolveVar accepts an exact resolved-variable name, or a structurally valid
// $lookup:<entity>.<op>:<key>=<flag>:<field> whose op exists and whose flag is declared.
func (d *Descriptor) checkResolveVar(v string, flagByName map[string]Flag) error {
	if cliResolveNames[v] {
		return nil
	}
	if strings.HasPrefix(v, "$lookup:") {
		parts := strings.Split(strings.TrimPrefix(v, "$lookup:"), ":")
		if len(parts) != 3 {
			return fmt.Errorf("malformed $lookup %q (want $lookup:<entity>.<op>:<key>=<flag>:<field>)", v)
		}
		ref, keyEq, field := parts[0], parts[1], parts[2]
		key, flag, ok := strings.Cut(keyEq, "=")
		if !ok || key == "" || flag == "" || field == "" {
			return fmt.Errorf("malformed $lookup %q (want $lookup:<entity>.<op>:<key>=<flag>:<field>)", v)
		}
		if !d.opExists(ref) {
			return fmt.Errorf("$lookup %q does not name an existing entity.op (%s)", v, ref)
		}
		if _, ok := flagByName[flag]; !ok {
			return fmt.Errorf("$lookup %q is keyed by unknown flag %q", v, flag)
		}
		return nil
	}
	names := make([]string, 0, len(cliResolveNames))
	for n := range cliResolveNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return fmt.Errorf("%q is not a supported resolved variable (want one of %s, or $lookup:<entity>.<op>:<key>=<flag>:<field>)", v, strings.Join(names, " "))
}

// walkTemplate classifies every leaf of the template: "$name"/"$name?" must be a flag's body
// var or a resolve entry (and "?" only on a flag that can be unset); any other scalar, or
// array of scalars, must sit under a key declared in Constants AND equal the declared value.
// inArray reports that the node sits inside a SINGLE-element array — the only place a
// repeatable flag's var may live, because the engine expands that element once per value —
// and elemRepeat records which repeatable var that element expands on, so a second one
// (whose expansion would be undefined) is rejected.
func walkTemplate(key string, v any, c *CLI, bodyVarByFlag map[string]Flag, seen map[string]bool, resolveOK func(string) error, inArray bool, elemRepeat *string) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := walkTemplate(k, t[k], c, bodyVarByFlag, seen, resolveOK, inArray, elemRepeat); err != nil {
				return err
			}
		}
		return nil
	case []any:
		single := len(t) == 1
		allLiteral := true
		for _, el := range t {
			if _, isObj := el.(map[string]any); isObj {
				allLiteral = false
			} else if s, ok := el.(string); ok && strings.HasPrefix(s, "$") {
				allLiteral = false
			}
		}
		if allLiteral {
			cv, declared := c.Constants[key]
			if !declared {
				return fmt.Errorf("body_template field %q is an unclassified example value (array) — map it to a flag, resolve it, or declare it in constants", key)
			}
			if !reflect.DeepEqual(cv, v) {
				return fmt.Errorf("body_template field %q = %v does not match the declared constant %v", key, v, cv)
			}
			return nil
		}
		for _, el := range t {
			var rep string
			if _, isObj := el.(map[string]any); isObj {
				if err := walkTemplate(key, el, c, bodyVarByFlag, seen, resolveOK, single, &rep); err != nil {
					return err
				}
				continue
			}
			if s, ok := el.(string); ok && strings.HasPrefix(s, "$") {
				if err := classifyVar(s, c, bodyVarByFlag, seen, resolveOK, single, &rep); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("body_template field %q mixes literal and variable array elements; declare the literal part as a constant or make every element a variable", key)
		}
		return nil
	case string:
		if strings.HasPrefix(t, "$") {
			return classifyVar(t, c, bodyVarByFlag, seen, resolveOK, inArray, elemRepeat)
		}
	}
	// A literal scalar leaf: declared, and equal to what was declared.
	cv, declared := c.Constants[key]
	if !declared {
		return fmt.Errorf("body_template field %q is an unclassified example value — map it to a flag, resolve it, or declare it in constants", key)
	}
	if !reflect.DeepEqual(cv, v) {
		return fmt.Errorf("body_template field %q = %v does not match the declared constant %v", key, v, cv)
	}
	return nil
}

func classifyVar(s string, c *CLI, bodyVarByFlag map[string]Flag, seen map[string]bool, resolveOK func(string) error, inArray bool, elemRepeat *string) error {
	optional := strings.HasSuffix(s, "?")
	name := strings.TrimSuffix(strings.TrimPrefix(s, "$"), "?")
	if f, ok := bodyVarByFlag[name]; ok {
		seen[name] = true
		if f.Repeatable {
			if !inArray {
				return fmt.Errorf("flag --%s is repeatable, so its var $%s must be inside a single-element array template (the element expands once per value)", f.Name, name)
			}
			if elemRepeat != nil {
				if *elemRepeat != "" && *elemRepeat != name {
					return fmt.Errorf("body_template array element already expands on repeatable --%s; a second repeatable --%s in the same element has no defined expansion", bodyVarByFlag[*elemRepeat].Name, f.Name)
				}
				*elemRepeat = name
			}
		}
		if optional && f.Required && !nulledBySomeFlag(f.Name, c) {
			return fmt.Errorf("body var $%s? is optional but flag --%s is required and nothing nulls it", name, f.Name)
		}
		return nil
	}
	if looksResolve("$" + name) {
		if err := resolveOK("$" + name); err != nil {
			return err // a mistyped or malformed resolved variable
		}
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

// looksResolve reports whether v is shaped like a resolved variable; validity is decided
// by checkResolveVar, this only routes the error message.
func looksResolve(v string) bool {
	for _, p := range cliResolveFamilies {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// spreadVars strips the body:$ prefix from spreads_to destinations.
func spreadVars(dests []string) []string {
	out := make([]string, 0, len(dests))
	for _, d := range dests {
		out = append(out, strings.TrimPrefix(strings.TrimPrefix(d, "body:"), "$"))
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, x := range a {
		seen[x] = true
	}
	for _, y := range b {
		if !seen[y] {
			return false
		}
	}
	return true
}

func joinKeys(m map[string][]string) string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
