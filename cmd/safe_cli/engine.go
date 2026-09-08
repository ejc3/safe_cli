package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/outfmt"
)

// verbCall is what a generated verb hands to invoke (docs/CLI-DESIGN.md §4): the op and
// the verb it was generated from, the flags the user EXPLICITLY gave (the generator emits
// pointer fields with no kong defaults, so absence is knowable — descriptor defaults are
// applied here), the target, and the global switches.
type verbCall struct {
	entity, op    string
	area, verb    string         // which of the op's verb blocks this is
	given         map[string]any // flag name -> parsed value, explicitly given flags only
	child         string         // --child SERVICE-ID ("" when not given)
	selfSvc       string         // the caller's own service id (from the id_token)
	selfPid       string         // the caller's own profile id (from the id_token)
	appUUID       string
	dryRun        bool
	confirm       bool
	allowUnpaired bool
}

// findVerb returns the op's verb block for area/verb (an op may back several verbs).
func findVerb(o descriptor.Operation, area, verb string) *descriptor.CLI {
	for _, b := range o.CLI {
		if b.Area == area && b.Verb == verb {
			return b
		}
	}
	return nil
}

// applyBranch returns a select branch's request contract: the verb with the branch's
// non-empty overrides (target, body_template, query, constants, resolve, headers) applied.
func applyBranch(c *descriptor.CLI, r descriptor.SelectRule) *descriptor.CLI {
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

// invoke is the one engine behind every generated verb: enforce the flag contract, resolve
// the target (one cached account read), pick the op by declared condition, guard an unpaired
// device, assemble the variables (given flags, descriptor defaults, transforms, spreads,
// nulls, resolved values, lookups), render the body/query/path/headers from the descriptor's
// templates, then --dry-run or send and render the output. Every request-shaping decision
// comes from the descriptor; nothing here special-cases an op.
func invoke(ctx context.Context, do doFunc, d *descriptor.Descriptor, vc verbCall, out io.Writer, asJSON bool) error {
	o, err := resolveOp(d, vc.entity, vc.op)
	if err != nil {
		return err
	}
	c := findVerb(o, vc.area, vc.verb)
	if c == nil {
		return fmt.Errorf("%s.%s has no generated verb %q %q; use `safe_cli call %s %s`", vc.entity, vc.op, vc.area, vc.verb, vc.entity, vc.op)
	}
	flagByName := make(map[string]descriptor.Flag, len(c.Flags))
	for _, f := range c.Flags {
		flagByName[f.Name] = f
	}
	given := vc.given
	if given == nil {
		given = map[string]any{}
	}
	// Dependent-flag contract, on explicitly given flags, before any request.
	for name := range given {
		for _, req := range flagByName[name].Requires {
			if _, ok := given[req]; !ok {
				return fmt.Errorf("--%s requires --%s", name, req)
			}
		}
	}
	if len(c.OneOf) > 0 {
		full := 0
		groups := make([]string, 0, len(c.OneOf))
		for _, grp := range c.OneOf {
			all := true
			for _, x := range grp {
				if _, ok := given[x]; !ok {
					all = false
				}
			}
			if all {
				full++
			}
			groups = append(groups, "--"+strings.Join(grp, " --"))
		}
		if full != 1 {
			return fmt.Errorf("%s %s needs exactly one of: %s", c.Area, c.Verb, strings.Join(groups, " | "))
		}
	}

	// Target: resolve --child whenever it was given (child/device verbs need it; a branch
	// may need it; exists: lookups run under it). The chosen contract decides whether it
	// was required.
	childGiven := strings.TrimSpace(vc.child) != ""
	var tgt *member
	var acct *account
	targetSvc := vc.selfSvc
	if childGiven {
		acct, err = fetchAccount(ctx, do, d, vc.selfSvc, vc.appUUID)
		if err != nil {
			return err
		}
		m, err := acct.resolveTarget(vc.child)
		if err != nil {
			return err
		}
		tgt = &m
		targetSvc = fmt.Sprintf("%d", m.ServiceID)
	}
	idHeaders := identityHeadersFrom(targetSvc, vc.selfPid, vc.appUUID, tgt, c.Target)

	// Conditional op selection, declared in the descriptor; the branch's contract applies.
	op, entity, merged := o, vc.entity, c
	if rule, ok, err := selectRule(ctx, do, d, c, given, childGiven, idHeaders); err != nil {
		return err
	} else if ok {
		ent, name, _ := strings.Cut(rule.Op, ".")
		op, err = resolveOp(d, ent, name)
		if err != nil {
			return err
		}
		entity = ent
		merged = applyBranch(c, rule)
	}
	if merged.Target == "child" || merged.Target == "device" {
		if tgt == nil {
			return fmt.Errorf("%s %s needs --child <SERVICE-ID> (run `safe_cli members`)", c.Area, c.Verb)
		}
		if merged.Target == "device" && !tgt.paired() && !vc.allowUnpaired {
			return fmt.Errorf("member %d (%s) is %s: %s %s has no effect until the child's phone is paired (safe_cli pairing show --child %d). Pass --allow-unpaired to send anyway",
				tgt.ServiceID, tgt.Name, strings.ToUpper(nonEmpty(tgt.Pairing, "UNPAIRED")), c.Area, c.Verb, tgt.ServiceID)
		}
	}
	idHeaders = identityHeadersFrom(targetSvc, vc.selfPid, vc.appUUID, tgt, merged.Target)
	if op.Destructive && !vc.confirm && !vc.dryRun {
		return fmt.Errorf("%s %s is catastrophic and effectively irreversible — re-run with --confirm", c.Area, c.Verb)
	}
	if merged.LiveEmergency && !vc.confirm && !vc.dryRun {
		return fmt.Errorf("%s %s triggers a LIVE emergency/dispatch flow on a real account — re-run with --confirm only if you mean it", c.Area, c.Verb)
	}

	// Variables: descriptor defaults under explicit values, transformed, spread, nulled.
	vars := make(map[string]any)
	queryVals := url.Values{}
	pathVals := map[string]string{}
	userHeaders := map[string]string{}
	for k, v := range merged.Headers { // fixed header constants first; flag headers override
		userHeaders[k] = v
	}
	repeat := map[string]bool{}
	for _, f := range merged.Flags {
		v, ok := given[f.Name]
		if !ok {
			v, err = defaultFor(f)
			if err != nil {
				return err
			}
		}
		if v == nil {
			continue
		}
		if f.Type == "enum" && !containsStr(f.Enum, fmt.Sprint(v)) {
			return fmt.Errorf("--%s %v is not one of %s", f.Name, v, strings.Join(f.Enum, "|"))
		}
		tv, err := applyTransform(f.Transform, v)
		if err != nil {
			return fmt.Errorf("--%s: %w", f.Name, err)
		}
		if len(f.SpreadsTo) > 0 {
			spread, ok := tv.(map[string]any)
			if !ok {
				return fmt.Errorf("--%s: structured transform %s must return an object", f.Name, f.Transform)
			}
			for _, dest := range f.SpreadsTo {
				name := strings.TrimPrefix(strings.TrimPrefix(dest, "body:"), "$")
				val, ok := spread[name]
				if !ok {
					return fmt.Errorf("--%s: transform %s returned no value for $%s", f.Name, f.Transform, name)
				}
				vars[name] = val
			}
			continue
		}
		kind, arg, _ := strings.Cut(f.MapsTo, ":")
		switch kind {
		case "body":
			name := strings.TrimPrefix(arg, "$")
			vars[name] = tv
			if f.Repeatable {
				repeat[name] = true
			}
		case "query":
			if !containsStr(op.Query, arg) {
				continue // declared for another branch's op only
			}
			s, err := queryString(tv)
			if err != nil {
				return fmt.Errorf("--%s: %w", f.Name, err)
			}
			queryVals.Set(arg, s)
		case "path":
			pathVals[arg] = fmt.Sprint(tv)
		case "header":
			userHeaders[arg] = fmt.Sprint(tv)
		}
	}
	// nulls: an explicitly given flag unsets the variables of the flags it nulls, so their
	// "$var?" properties are omitted (pause --indefinite drops pauseSchedule).
	for name := range given {
		for _, x := range flagByName[name].Nulls {
			if _, arg, _ := strings.Cut(flagByName[x].MapsTo, ":"); arg != "" {
				delete(vars, strings.TrimPrefix(arg, "$"))
			}
		}
	}
	if len(merged.AtLeastOne) > 0 {
		oneGiven := false
		for _, x := range merged.AtLeastOne {
			if _, ok := given[x]; ok {
				oneGiven = true
			}
		}
		if !oneGiven {
			return fmt.Errorf("%s %s needs at least one of --%s", c.Area, c.Verb, strings.Join(merged.AtLeastOne, ", --"))
		}
	}

	// Resolved values the user never types.
	if err := fillResolved(ctx, do, d, merged, vars, tgt, acct, vc, idHeaders, given); err != nil {
		return err
	}

	// Path: placeholders from flags, then the target's ids for {deviceId}/{profileId}/{serviceId}.
	if tgt != nil {
		for ph, v := range map[string]int64{"deviceId": tgt.DeviceID, "profileId": tgt.ProfileID, "serviceId": tgt.ServiceID} {
			if _, set := pathVals[ph]; !set && strings.Contains(op.Path, "{"+ph+"}") && v != 0 {
				pathVals[ph] = fmt.Sprintf("%d", v)
			}
		}
	}
	ent, _ := d.Entity(entity)
	path, err := fillPath(op.Path, ent.IDField, "", pathVals)
	if err != nil {
		return err
	}
	// Query: declared constants/"$flag" values plus flag-mapped params.
	q, err := renderQuery(merged.Query, flagValues(given, flagByName))
	if err != nil {
		return err
	}
	for k, vs := range queryVals {
		for _, v := range vs {
			if q == nil {
				q = url.Values{}
			}
			q.Add(k, v)
		}
	}
	path = appendQuery(path, q)
	// Body.
	var body []byte
	if merged.BodyTemplate != "" {
		body, err = renderTemplateRepeat(merged.BodyTemplate, vars, repeat)
		if err != nil {
			return err
		}
	}
	headers, missingSvc, err := assembleHeaders(op, idHeaders, userHeaders)
	if err != nil {
		return err
	}
	if len(missingSvc) > 0 {
		return fmt.Errorf("%s %s: no service id for %s (internal: target resolution left it empty)", c.Area, c.Verb, strings.Join(missingSvc, ", "))
	}
	resp, err := do(ctx, op.Method, path, body, headers)
	if err != nil {
		return err
	}
	if vc.dryRun {
		return writeDryRun(out, asJSON, resp, tgt)
	}
	return writeVerbResponse(out, asJSON, resp, merged, tgt)
}

func nonEmpty(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

// identityHeadersFrom builds the x-fp-identifier-* values for the request: the target's
// service id (or the caller's own for account/self verbs), the caller's profile id, and
// the install's app-uuid. Device verbs also carry the target's device id.
func identityHeadersFrom(targetSvc, selfPid, appUUID string, tgt *member, target string) map[string]string {
	m := map[string]string{}
	if targetSvc != "" {
		m["x-fp-identifier-target-serviceid"] = targetSvc
	}
	if selfPid != "" {
		m["x-fp-identifier-profileid"] = selfPid
	}
	if tgt != nil && target == "device" && tgt.DeviceID != 0 {
		m["x-fp-identifier-deviceid"] = fmt.Sprintf("%d", tgt.DeviceID)
	}
	if appUUID != "" {
		m["x-fp-identifier-app-uuid"] = appUUID
	}
	return m
}

// defaultFor applies a flag's descriptor default. A literal is used as-is; a "$..." default
// is a resolved value (only $local.timezone is a defaultable resolved value today).
func defaultFor(f descriptor.Flag) (any, error) {
	if f.Default == nil {
		return nil, nil
	}
	if s, ok := f.Default.(string); ok && strings.HasPrefix(s, "$") {
		switch s {
		case "$local.timezone":
			return localTimezone(), nil
		}
		return nil, fmt.Errorf("--%s: default %q is not a resolvable value", f.Name, s)
	}
	return f.Default, nil
}

// localTimezone is the CLI host's zone — the grounded default for --timezone, since no op
// reads the account's zone back (docs/CLI-DESIGN.md §3).
func localTimezone() string {
	name := time.Local.String()
	if name == "" || name == "Local" {
		return time.Now().Format("MST")
	}
	return name
}

// fillResolved fills the variables the engine resolves itself, from the resolve list.
func fillResolved(ctx context.Context, do doFunc, d *descriptor.Descriptor, c *descriptor.CLI, vars map[string]any, tgt *member, acct *account, vc verbCall, idHeaders map[string]string, given map[string]any) error {
	for _, r := range c.Resolve {
		name := strings.TrimPrefix(r, "$")
		switch {
		case strings.HasPrefix(r, "$child."):
			if tgt == nil {
				return fmt.Errorf("%s needs a --child target", r)
			}
			switch r {
			case "$child.serviceId":
				vars[name] = tgt.ServiceID
			case "$child.profileId":
				vars[name] = tgt.ProfileID
			case "$child.deviceId":
				vars[name] = tgt.DeviceID
			case "$child.pairing":
				vars[name] = tgt.Pairing
			}
		case r == "$self.serviceId":
			vars[name] = jsonNumber(vc.selfSvc)
		case r == "$self.profileId":
			vars[name] = jsonNumber(vc.selfPid)
		case r == "$account.id":
			if acct == nil {
				a, err := fetchAccount(ctx, do, d, vc.selfSvc, vc.appUUID)
				if err != nil {
					return err
				}
				acct = a
			}
			vars[name] = acct.ID
		case r == "$local.timezone":
			vars[name] = localTimezone()
		case r == "$now.epochMs":
			vars[name] = time.Now().UnixMilli()
		case r == "$uuid":
			u, err := client.TraceID()
			if err != nil {
				return err
			}
			vars[name] = u
		case strings.HasPrefix(r, "$lookup:"):
			v, found, err := runLookup(ctx, do, d, r, given, idHeaders)
			if err != nil {
				return err
			}
			if !found {
				return lookupMiss(r, given)
			}
			vars[name] = v
		}
	}
	return nil
}

// jsonNumber turns a numeric id string into a number for the body (ids are numbers on the
// wire); a non-numeric value stays a string.
func jsonNumber(s string) any {
	n := json.Number(s)
	if i, err := n.Int64(); err == nil {
		return i
	}
	return s
}

// selectRule evaluates cli.select in order and returns the first matching rule, if any.
func selectRule(ctx context.Context, do doFunc, d *descriptor.Descriptor, c *descriptor.CLI, given map[string]any, childGiven bool, idHeaders map[string]string) (descriptor.SelectRule, bool, error) {
	for _, r := range c.Select {
		switch {
		case r.When == "child":
			if childGiven {
				return r, true, nil
			}
		case strings.HasPrefix(r.When, "flag:"):
			if _, ok := given[strings.TrimPrefix(r.When, "flag:")]; ok {
				return r, true, nil
			}
		case strings.HasPrefix(r.When, "exists:"):
			_, found, err := runLookup(ctx, do, d, strings.TrimPrefix(r.When, "exists:"), given, idHeaders)
			if err != nil {
				return descriptor.SelectRule{}, false, err
			}
			if found {
				return r, true, nil
			}
		}
	}
	return descriptor.SelectRule{}, false, nil
}

// runLookup performs a $lookup:<entity>.<op>:<key>=<flag>:<field> enrichment read: GET the
// op with the target headers, find the first object whose <key> equals the flag's value,
// and return its <field>. found=false when no record matches.
func runLookup(ctx context.Context, do doFunc, d *descriptor.Descriptor, spec string, given map[string]any, idHeaders map[string]string) (any, bool, error) {
	parts := strings.Split(strings.TrimPrefix(spec, "$lookup:"), ":")
	if len(parts) != 3 {
		return nil, false, fmt.Errorf("malformed lookup %q", spec)
	}
	ref, keyEq, field := parts[0], parts[1], parts[2]
	key, flag, _ := strings.Cut(keyEq, "=")
	want, ok := given[flag]
	if !ok {
		return nil, false, fmt.Errorf("lookup %s needs --%s", ref, flag)
	}
	ent, name, _ := strings.Cut(ref, ".")
	op, err := resolveOp(d, ent, name)
	if err != nil {
		return nil, false, err
	}
	headers, _, err := assembleHeaders(op, idHeaders, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := do(ctx, op.Method, op.Path, nil, headers)
	if err != nil {
		return nil, false, err
	}
	if resp.Status >= 400 {
		return nil, false, fmt.Errorf("lookup %s: HTTP %d: %s", ref, resp.Status, strings.TrimSpace(string(resp.Body)))
	}
	var doc any
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, false, fmt.Errorf("lookup %s: %w", ref, err)
	}
	rec := findRecord(doc, key, fmt.Sprint(want))
	if rec == nil {
		return nil, false, nil
	}
	v, ok := rec[field]
	if !ok {
		return nil, false, fmt.Errorf("lookup %s: matching record has no field %q", ref, field)
	}
	return v, true, nil
}

// findRecord walks a JSON document for the first object whose key equals want (compared
// as text, so a numeric id matches "10003").
func findRecord(n any, key, want string) map[string]any {
	switch t := n.(type) {
	case map[string]any:
		if v, ok := t[key]; ok && fmt.Sprint(normalizeNum(v)) == want {
			return t
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if r := findRecord(t[k], key, want); r != nil {
				return r
			}
		}
	case []any:
		for _, el := range t {
			if r := findRecord(el, key, want); r != nil {
				return r
			}
		}
	}
	return nil
}

// normalizeNum prints a JSON float that is a whole number without a decimal part so
// 10003 (float64 from encoding/json) compares equal to the flag text "10003".
func normalizeNum(v any) any {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f)
	}
	return v
}

func lookupMiss(spec string, given map[string]any) error {
	parts := strings.Split(strings.TrimPrefix(spec, "$lookup:"), ":")
	ref := parts[0]
	key, flag, _ := strings.Cut(parts[1], "=")
	ent, name, _ := strings.Cut(ref, ".")
	return fmt.Errorf("no %s record with %s=%v; run `safe_cli call %s %s` to list valid ids", ref, key, given[flag], ent, name)
}

// flagValues exposes flag values by FLAG name for renderQuery's "$flag" references (vars is
// keyed by body var name, which may differ).
func flagValues(given map[string]any, flagByName map[string]descriptor.Flag) map[string]any {
	out := make(map[string]any, len(given))
	for name, f := range flagByName {
		if v, ok := given[name]; ok {
			tv, err := applyTransform(f.Transform, v)
			if err == nil {
				out[name] = tv
			}
		}
	}
	return out
}

// writeDryRun prints the exact request (as dumpRequest built it) plus the resolved target,
// so an agent can see which ids were filled in before sending for real.
func writeDryRun(out io.Writer, asJSON bool, resp *client.Response, tgt *member) error {
	if asJSON {
		var m map[string]any
		if err := json.Unmarshal(resp.Body, &m); err != nil {
			m = map[string]any{"request": string(resp.Body)}
		}
		if tgt != nil {
			m["resolved"] = tgt
		}
		return outfmt.JSON(out, m)
	}
	if tgt != nil {
		if _, err := fmt.Fprintf(out, "# target: %s (service %d, profile %d, device %d, %s)\n", tgt.Name, tgt.ServiceID, tgt.ProfileID, tgt.DeviceID, tgt.Pairing); err != nil {
			return err
		}
	}
	_, err := out.Write(ensureNewline(resp.Body))
	return err
}

// writeVerbResponse renders the backend response: raw JSON under --json (with a _meta of
// the resolved target so an agent can chain), otherwise the fields cli.output.table names
// as a one-row table, falling back to pretty JSON.
func writeVerbResponse(out io.Writer, asJSON bool, resp *client.Response, c *descriptor.CLI, tgt *member) error {
	if resp.Status >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.Status, strings.TrimSpace(string(resp.Body)))
	}
	if asJSON {
		var m map[string]any
		if err := json.Unmarshal(resp.Body, &m); err == nil && tgt != nil {
			m["_meta"] = map[string]any{"target": tgt}
			return outfmt.JSON(out, m)
		}
		_, err := out.Write(ensureNewline(resp.Body))
		return err
	}
	if c.Output != nil && len(c.Output.Table) > 0 {
		var m map[string]any
		if err := json.Unmarshal(resp.Body, &m); err == nil {
			row := make([]string, 0, len(c.Output.Table))
			hit := false
			for _, col := range c.Output.Table {
				v := digField(m, col)
				if v != nil {
					hit = true
				}
				row = append(row, fmt.Sprint(nilToDash(v)))
			}
			if hit {
				return outfmt.Table(out, upper(c.Output.Table), [][]string{row})
			}
		}
	}
	return writeAPIResponse(out, false, resp)
}

// digField finds a named field at the top level or one level down (e.g. devices[0].status).
func digField(m map[string]any, name string) any {
	if v, ok := m[name]; ok {
		return normalizeNum(v)
	}
	for _, v := range m {
		switch t := v.(type) {
		case map[string]any:
			if x, ok := t[name]; ok {
				return normalizeNum(x)
			}
		case []any:
			if len(t) > 0 {
				if obj, ok := t[0].(map[string]any); ok {
					if x, ok := obj[name]; ok {
						return normalizeNum(x)
					}
				}
			}
		}
	}
	return nil
}

func nilToDash(v any) any {
	if v == nil {
		return "-"
	}
	return v
}

func upper(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strings.ToUpper(x)
	}
	return out
}
