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
	entity, op  string
	area, verb  string         // which of the op's verb blocks this is
	given       map[string]any // flag name -> parsed value, explicitly given flags only
	child       string         // --child SERVICE-ID ("" when not given)
	selfSvc     string         // the caller's own service id (from the id_token)
	selfPid     string         // the caller's own profile id (from the id_token)
	appUUID     string
	sessionUUID string // the token set's own app-uuid, for body injection; never the install fallback
	dryRun      bool
	// dump, when set with dryRun, replaces do for the FINAL request only: the account read
	// and lookups stay real (they resolve the ids the dump shows), the verb's own request
	// is rendered and printed, never sent.
	dump          doFunc
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
	// Two notions of presence. A value contract (requires, excludes, one_of, at_least_one)
	// sees every explicitly given flag, --flag=false included: a value-style bool such as
	// --objectionable-alerts=false IS the setting. A switch (select flag:, nulls) fires only
	// when the flag is asserted: --flag=false is the switch's absence.
	present := given
	// keyVals is what a keyed lookup may be keyed by: the given flags plus every literal
	// default (the schema accepts a defaulted key flag as always present), each carried as
	// its wire value — after the flag's transform — because that is what the looked-up
	// document holds. A transform that fails here is reported by the flag loop below.
	keyVals := map[string]any{}
	for name, v := range given {
		keyVals[name] = v
	}
	for _, f := range c.Flags {
		if _, ok := keyVals[f.Name]; !ok && f.Default != nil {
			if ds, isStr := f.Default.(string); isStr && strings.HasPrefix(ds, "$") {
				continue // a resolved default is not a lookup key
			}
			keyVals[f.Name] = normalizeNum(f.Default)
		}
		if v, ok := keyVals[f.Name]; ok && f.Transform != "" {
			if tv, err := applyTransform(f.Transform, v); err == nil {
				keyVals[f.Name] = tv
			}
		}
	}
	asserted := map[string]any{}
	for name, v := range given {
		if b, isBool := v.(bool); isBool && !b {
			continue
		}
		asserted[name] = v
	}
	// Dependent-flag contract, on explicitly given flags, before any request.
	for name := range present {
		for _, req := range flagByName[name].Requires {
			if _, ok := present[req]; !ok {
				return fmt.Errorf("--%s requires --%s", name, req)
			}
		}
		for _, x := range flagByName[name].Excludes {
			if _, ok := present[x]; ok {
				return fmt.Errorf("--%s and --%s cannot be combined", name, x)
			}
		}
	}
	if len(c.OneOf) > 0 {
		full := 0
		groups := make([]string, 0, len(c.OneOf))
		for _, grp := range c.OneOf {
			all := true
			for _, x := range grp {
				if _, ok := present[x]; !ok {
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
		// The one complete group is the choice; a stray member of another alternative would
		// silently mix two request shapes.
		var chosen []string
		for _, grp := range c.OneOf {
			all := true
			for _, x := range grp {
				if _, ok := present[x]; !ok {
					all = false
				}
			}
			if all {
				chosen = grp
			}
		}
		for gi, grp := range c.OneOf {
			for _, x := range grp {
				if _, ok := present[x]; ok && !containsStr(chosen, x) {
					return fmt.Errorf("--%s belongs to the %s alternative, not to the one given (--%s); pass one alternative only", x, "--"+strings.Join(grp, " --"), strings.Join(chosen, " --"))
				}
			}
			_ = gi
		}
	}
	// Enum values and at_least_one are structural too: refuse them here, before the
	// account read, rather than after a network round trip.
	for name, v := range given {
		if err := checkEnum(flagByName[name], v); err != nil {
			return err
		}
	}
	if len(c.AtLeastOne) > 0 {
		oneGiven := false
		for _, x := range c.AtLeastOne {
			if _, ok := present[x]; ok {
				oneGiven = true
			}
		}
		if !oneGiven {
			return fmt.Errorf("%s %s needs at least one of --%s", c.Area, c.Verb, strings.Join(c.AtLeastOne, ", --"))
		}
	}
	// Enum values and at_least_one are structural too: refuse them here, before the
	// account read, rather than after a network round trip.
	for name, v := range given {
		if err := checkEnum(flagByName[name], v); err != nil {
			return err
		}
	}
	if len(c.AtLeastOne) > 0 {
		oneGiven := false
		for _, x := range c.AtLeastOne {
			if _, ok := present[x]; ok {
				oneGiven = true
			}
		}
		if !oneGiven {
			return fmt.Errorf("%s %s needs at least one of --%s", c.Area, c.Verb, strings.Join(c.AtLeastOne, ", --"))
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
	lookups := lookupCache{} // one read per lookup per invocation: select and render share a snapshot
	if rule, ok, err := selectRule(ctx, do, d, c, asserted, keyVals, childGiven, idHeaders, lookups); err != nil {
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
	if merged.Target != "child" && merged.Target != "device" {
		targetSvc = vc.selfSvc // an account/self verb acts on the caller's own service even when --child filters its output
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
	repeat := map[string]bool{}
	filters := map[string]any{}   // response field -> value, for filter: flags
	effective := map[string]any{} // flag name -> its transformed value, given or defaulted, for "$flag" query refs
	resolveDefault := func(spec string) (any, error) {
		switch {
		case spec == "$local.timezone":
			return localTimezone(), nil
		case strings.HasPrefix(spec, "$lookup:"):
			v, found, err := runLookup(ctx, do, d, spec, keyVals, idHeaders, lookups)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, nil // nothing current to resend; the flag stays unset
			}
			return v, nil
		}
		return nil, fmt.Errorf("default %q is not a resolvable value", spec)
	}
	for _, f := range merged.Flags {
		v, ok := given[f.Name]
		if !ok {
			v, err = defaultFor(f, resolveDefault)
			if err != nil {
				return fmt.Errorf("--%s: %w", f.Name, err)
			}
		}
		if f.Repeatable { // even when absent: the singleton array expands to [] rather than [{}]
			if _, arg, ok := strings.Cut(f.MapsTo, ":"); ok && strings.HasPrefix(arg, "$") {
				repeat[strings.TrimPrefix(arg, "$")] = true
			}
		}
		if v == nil {
			continue
		}
		if err := checkEnum(f, v); err != nil {
			return err
		}
		tv, err := applyTransform(f.Transform, v)
		if err != nil {
			return fmt.Errorf("--%s: %w", f.Name, err)
		}
		effective[f.Name] = tv
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
				if ok { // explicitly given: never silently drop it
					return fmt.Errorf("--%s is only accepted with %s", f.Name, branchCondition(d, c, "query", arg))
				}
				continue // a default for another branch's op
			}
			s, err := queryString(tv)
			if err != nil {
				return fmt.Errorf("--%s: %w", f.Name, err)
			}
			queryVals.Set(arg, s)
		case "path":
			pathVals[arg] = fmt.Sprint(normalizeNum(tv))
		case "header":
			if !containsStr(op.Headers, arg) {
				if ok { // explicitly given: never silently drop it
					return fmt.Errorf("--%s is only accepted with %s", f.Name, branchCondition(d, c, "header", arg))
				}
				continue // a default for another branch's op
			}
			userHeaders[arg] = fmt.Sprint(normalizeNum(tv))
		case "filter":
			if ok { // only an explicitly given selector filters
				filters[arg] = tv
			}
		}
	}
	// nulls: an explicitly given flag unsets the variables of the flags it nulls, so their
	// "$var?" properties are omitted (pause --indefinite drops pauseSchedule).
	for name := range asserted { // a false bool is a switch not thrown: it nulls nothing
		for _, x := range flagByName[name].Nulls {
			if _, arg, _ := strings.Cut(flagByName[x].MapsTo, ":"); arg != "" {
				delete(vars, strings.TrimPrefix(arg, "$"))
			}
			delete(effective, x)
		}
	}

	// Resolved values the user never types.
	if err := fillResolved(ctx, do, d, merged, vars, tgt, acct, vc, idHeaders, keyVals, lookups); err != nil {
		return err
	}
	// Fixed header constants, or resolver variables ($local.timezone for a contextual
	// header); a flag-mapped header keeps precedence.
	for k, v := range merged.Headers {
		if _, fromFlag := userHeaders[k]; fromFlag {
			continue
		}
		if strings.HasPrefix(v, "$") {
			rv, ok := vars[strings.TrimPrefix(v, "$")]
			if !ok || rv == nil {
				return fmt.Errorf("header %s: %s has no value", k, v)
			}
			userHeaders[k] = fmt.Sprint(rv)
			continue
		}
		userHeaders[k] = v
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
	q, err := renderQuery(merged.Query, withResolved(effective, vars))
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
	if missing := missingRequiredQuery(op.RequiredQuery, q); len(missing) > 0 {
		return fmt.Errorf("%s %s: required query param(s) %v have no value (an empty flag, or a lookup that found nothing); nothing was sent", c.Area, c.Verb, missing)
	}
	path = appendQuery(path, q)
	// Body.
	var body []byte
	if merged.BodyTemplate != "" {
		body, err = renderTemplateRepeat(merged.BodyTemplate, vars, repeat)
		if err != nil {
			return err
		}
		if op.InjectCallerAppUUID { // the caller's own session uuid, as `call` injects it
			if vc.sessionUUID == "" {
				return fmt.Errorf("%s %s fills the body's app_uuid with this session's app-uuid, and the stored token set has none (imported without app_uuid); re-import it with app_uuid or run `safe_cli auth login`. Nothing was sent", c.Area, c.Verb)
			}
			if body, err = injectAppUUID(body, vc.sessionUUID); err != nil {
				return err
			}
		}
	}
	headers, missingSvc, err := assembleHeaders(op, idHeaders, userHeaders)
	if err != nil {
		return err
	}
	if len(missingSvc) > 0 {
		return fmt.Errorf("%s %s: no service id for %s (internal: target resolution left it empty)", c.Area, c.Verb, strings.Join(missingSvc, ", "))
	}
	send := do
	if vc.dryRun {
		// A dry run must never reach the live sender: without a dumper there is nothing
		// safe to call, so refuse rather than fall through.
		if vc.dump == nil {
			return fmt.Errorf("--dry-run: no request dumper was supplied (internal error); nothing was sent")
		}
		send = vc.dump
	}
	resp, err := send(ctx, op.Method, path, body, headers)
	if err != nil {
		return err
	}
	if vc.dryRun {
		return writeDryRun(out, asJSON, resp, tgt)
	}
	if len(filters) > 0 && resp.Status < 400 {
		resp.Body = filterResponse(resp.Body, filters)
	}
	return writeVerbResponse(out, asJSON, resp, merged, tgt)
}

// withResolved returns the flag values plus the resolved variables, keyed by var name, so
// a query map may reference either ("$since" or "$child.profileId").
func withResolved(flagVals, vars map[string]any) map[string]any {
	out := make(map[string]any, len(flagVals)+len(vars))
	for k, v := range vars {
		out[k] = v
	}
	for k, v := range flagVals {
		out[k] = v
	}
	return out
}

// filterResponse applies filter: selectors client-side: every array of objects in the
// response keeps only the objects whose field equals the selector value (compared as text,
// so 2000001 matches "2000001"). The request itself was never narrowed.
func filterResponse(body []byte, filters map[string]any) []byte {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return body
	}
	out, err := json.Marshal(filterNode(doc, filters))
	if err != nil {
		return body
	}
	return out
}

func filterNode(n any, filters map[string]any) any {
	switch t := n.(type) {
	case map[string]any:
		for k, v := range t {
			t[k] = filterNode(v, filters)
		}
		return t
	case []any:
		kept := make([]any, 0, len(t))
		for _, el := range t {
			obj, isObj := el.(map[string]any)
			if isObj && !matchesFilters(obj, filters) {
				continue
			}
			kept = append(kept, filterNode(el, filters))
		}
		return kept
	}
	return n
}

func matchesFilters(obj map[string]any, filters map[string]any) bool {
	for field, want := range filters {
		got, ok := obj[field]
		if !ok {
			continue // an object without the field is not a candidate row; keep it
		}
		if fmt.Sprint(normalizeNum(got)) != fmt.Sprint(normalizeNum(want)) {
			return false
		}
	}
	return true
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

// defaultFor applies a flag's descriptor default: a literal as-is, or a "$..." default
// resolved by resolve (a resolver variable such as $local.timezone, or an unkeyed lookup
// that resends an untouched field's current value).
func defaultFor(f descriptor.Flag, resolve func(string) (any, error)) (any, error) {
	if f.Default == nil {
		return nil, nil
	}
	if s, ok := f.Default.(string); ok && strings.HasPrefix(s, "$") {
		return resolve(s)
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
func fillResolved(ctx context.Context, do doFunc, d *descriptor.Descriptor, c *descriptor.CLI, vars map[string]any, tgt *member, acct *account, vc verbCall, idHeaders map[string]string, given map[string]any, lookups lookupCache) error {
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
			v, found, err := runLookup(ctx, do, d, r, given, idHeaders, lookups)
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
func selectRule(ctx context.Context, do doFunc, d *descriptor.Descriptor, c *descriptor.CLI, given, keyVals map[string]any, childGiven bool, idHeaders map[string]string, lookups lookupCache) (descriptor.SelectRule, bool, error) {
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
			spec := strings.TrimPrefix(r.When, "exists:")
			if key := lookupKeyFlag(spec); key != "" {
				if _, ok := keyVals[key]; !ok {
					continue // keyed by a flag that was not given (and has no default): the record cannot exist for us
				}
			}
			_, found, err := runLookup(ctx, do, d, spec, keyVals, idHeaders, lookups)
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

// lookupCache memoizes the decoded document of every lookup op read during one invocation,
// keyed by the op and the target it was read for, so an exists: condition and however many
// resolve entries name the same op share one read (and one snapshot).
type lookupCache map[string]any

// runLookup performs a $lookup:<entity>.<op>:<key>=<flag>:<field> enrichment read: GET the
// op with the target headers (once per invocation), find the first object whose <key>
// equals the flag's value, and return its <field>. found=false when no record matches.
func runLookup(ctx context.Context, do doFunc, d *descriptor.Descriptor, spec string, given map[string]any, idHeaders map[string]string, lookups lookupCache) (any, bool, error) {
	parts := strings.Split(strings.TrimPrefix(spec, "$lookup:"), ":")
	if len(parts) != 3 {
		return nil, false, fmt.Errorf("malformed lookup %q", spec)
	}
	ref, keyEq, field := parts[0], parts[1], parts[2]
	// "entity.op/field" searches only that top-level subtree of the read.
	ref, subtree, _ := strings.Cut(ref, "/")
	cacheKey := ref + "\x00" + idHeaders["x-fp-identifier-target-serviceid"]
	doc, ok := lookups[cacheKey]
	if !ok {
		var err error
		doc, err = readLookupDoc(ctx, do, d, ref, idHeaders)
		if err != nil {
			return nil, false, err
		}
		if lookups != nil {
			lookups[cacheKey] = doc
		}
	}
	if subtree != "" {
		m, _ := doc.(map[string]any)
		doc = m[subtree] // absent: nothing to find
	}
	return extractLookup(doc, ref, keyEq, field, given)
}

// readLookupDoc issues the lookup op's bare GET with the target headers and decodes it.
func readLookupDoc(ctx context.Context, do doFunc, d *descriptor.Descriptor, ref string, idHeaders map[string]string) (any, error) {
	ent, name, _ := strings.Cut(ref, ".")
	op, err := resolveOp(d, ent, name)
	if err != nil {
		return nil, err
	}
	headers, _, err := assembleHeaders(op, idHeaders, nil)
	if err != nil {
		return nil, err
	}
	resp, err := do(ctx, op.Method, op.Path, nil, headers)
	if err != nil {
		return nil, err
	}
	if resp.Status >= 400 {
		return nil, fmt.Errorf("lookup %s: HTTP %d: %s", ref, resp.Status, strings.TrimSpace(string(resp.Body)))
	}
	var doc any
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		return nil, fmt.Errorf("lookup %s: %w", ref, err)
	}
	return doc, nil
}

// extractLookup finds the record a lookup spec names inside a decoded document.
func extractLookup(doc any, ref, keyEq, field string, given map[string]any) (any, bool, error) {
	var key, flag string
	var want any
	keyed := keyEq != ""
	if keyed {
		key, flag, _ = strings.Cut(keyEq, "=")
		var ok bool
		want, ok = given[flag]
		if !ok {
			return nil, false, fmt.Errorf("lookup %s needs --%s", ref, flag)
		}
	}
	var rec map[string]any
	if keyed {
		rec = findRecord(doc, key, fmt.Sprint(want))
	} else {
		rec = singletonRecord(doc, field)
	}
	if rec == nil {
		return nil, false, nil
	}
	v, ok := rec[field]
	if !ok || v == nil {
		if keyed {
			return nil, false, fmt.Errorf("lookup %s: matching record has no field %q", ref, field)
		}
		return nil, false, nil // the singleton exists but carries no such field: nothing to resend/select
	}
	return v, true, nil
}

// singletonRecord is the target's one record for an unkeyed lookup: the first object,
// walking the response depth-first, that carries field — so a wrapped read such as
// {"accounts":[{"familyName":...}]} yields the account, not the wrapper.
func singletonRecord(doc any, field string) map[string]any {
	switch t := doc.(type) {
	case map[string]any:
		if _, ok := t[field]; ok {
			return t
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if r := singletonRecord(t[k], field); r != nil {
				return r
			}
		}
	case []any:
		for _, el := range t {
			if r := singletonRecord(el, field); r != nil {
				return r
			}
		}
	}
	return nil
}

// lookupKeyFlag returns the flag a keyed $lookup spec is keyed by ("" when unkeyed).
func lookupKeyFlag(spec string) string {
	parts := strings.Split(strings.TrimPrefix(spec, "$lookup:"), ":")
	if len(parts) != 3 || parts[1] == "" {
		return ""
	}
	_, flag, _ := strings.Cut(parts[1], "=")
	return flag
}

// checkEnum refuses a value (or, for a repeatable enum, any element) outside the flag's enum.
func checkEnum(f descriptor.Flag, v any) error {
	if f.Type != "enum" {
		return nil
	}
	vals := []string{}
	switch t := v.(type) {
	case []string:
		vals = t
	case []any:
		for _, el := range t {
			vals = append(vals, fmt.Sprint(el))
		}
	default:
		vals = append(vals, fmt.Sprint(v))
	}
	for _, s := range vals {
		if !containsStr(f.Enum, s) {
			return fmt.Errorf("--%s %v is not one of %s", f.Name, s, strings.Join(f.Enum, "|"))
		}
	}
	return nil
}

// branchCondition names, for an error, what selects a branch whose op takes query param
// arg: "--child", "--flag", or "an existing entity.op record".
func branchCondition(d *descriptor.Descriptor, c *descriptor.CLI, kind, arg string) string {
	var conds []string
	for _, r := range c.Select {
		ent, name, _ := strings.Cut(r.Op, ".")
		bo, err := resolveOp(d, ent, name)
		if err != nil {
			continue
		}
		declared := bo.Query
		if kind == "header" {
			declared = bo.Headers
		}
		if !containsStr(declared, arg) {
			continue
		}
		switch {
		case r.When == "child":
			conds = append(conds, "--child")
		case strings.HasPrefix(r.When, "flag:"):
			conds = append(conds, "--"+strings.TrimPrefix(r.When, "flag:"))
		case strings.HasPrefix(r.When, "exists:"):
			ref := strings.Split(strings.TrimPrefix(r.When, "exists:$lookup:"), ":")[0]
			conds = append(conds, "an existing "+ref+" record")
		}
	}
	if len(conds) == 0 {
		return "another branch of this verb"
	}
	return strings.Join(conds, " or ")
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

func lookupMiss(spec string, given map[string]any) error {
	parts := strings.Split(strings.TrimPrefix(spec, "$lookup:"), ":")
	ref := parts[0]
	opRef, _, _ := strings.Cut(ref, "/") // the hint names the op, not the searched subtree
	ent, name, _ := strings.Cut(opRef, ".")
	if parts[1] == "" {
		return fmt.Errorf("%s has no %s for this target; run `safe_cli call %s %s` to see it", ref, parts[2], ent, name)
	}
	key, flag, _ := strings.Cut(parts[1], "=")
	return fmt.Errorf("no %s record with %s=%v; run `safe_cli call %s %s` to list valid ids", ref, key, given[flag], ent, name)
}

// writeDryRun prints the exact request (as dumpRequest built it) plus the resolved target,
// so an agent can see which ids were filled in before sending for real.
func writeDryRun(out io.Writer, asJSON bool, resp *client.Response, tgt *member) error {
	if asJSON {
		var m map[string]any
		if err := json.Unmarshal(resp.Body, &m); err != nil || m == nil {
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
			if m == nil { // a JSON null body decodes to a nil map
				m = map[string]any{}
			}
			m["_meta"] = map[string]any{"target": tgt}
			return outfmt.JSON(out, m)
		}
		var v any
		if err := json.Unmarshal(resp.Body, &v); err == nil && tgt != nil {
			// An array or scalar body: keep it whole under "data" so _meta still travels.
			return outfmt.JSON(out, map[string]any{"data": v, "_meta": map[string]any{"target": tgt}})
		}
		_, err := out.Write(ensureNewline(resp.Body))
		return err
	}
	if c.Output != nil && len(c.Output.Table) > 0 {
		var doc any
		if err := json.Unmarshal(resp.Body, &doc); err == nil {
			var rows [][]string
			hit := false
			for _, rec := range tableRecords(doc) {
				row := make([]string, 0, len(c.Output.Table))
				for _, col := range c.Output.Table {
					v := digField(rec, col)
					if v != nil {
						hit = true
					}
					row = append(row, fmt.Sprint(nilToDash(v)))
				}
				rows = append(rows, row)
			}
			if hit {
				return outfmt.Table(out, upper(c.Output.Table), rows)
			}
		}
	}
	return writeAPIResponse(out, false, resp)
}

// tableRecords picks the objects a table row stands for: the elements of a top-level
// array, the elements of an object's one array-of-objects field (a wrapped listing such
// as {"devices":[...]}), or else the object itself.
func tableRecords(doc any) []map[string]any {
	switch t := doc.(type) {
	case []any:
		var recs []map[string]any
		for _, el := range t {
			if m, ok := el.(map[string]any); ok {
				recs = append(recs, m)
			}
		}
		if len(recs) > 0 {
			return recs
		}
	case map[string]any:
		var listFields [][]map[string]any
		for _, v := range t {
			arr, ok := v.([]any)
			if !ok || len(arr) == 0 {
				continue
			}
			var recs []map[string]any
			for _, el := range arr {
				if m, ok := el.(map[string]any); ok {
					recs = append(recs, m)
				}
			}
			if len(recs) == len(arr) {
				listFields = append(listFields, recs)
			}
		}
		if len(listFields) == 1 {
			return listFields[0]
		}
		return []map[string]any{t}
	}
	return nil
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
