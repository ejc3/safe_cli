package descriptor

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// emptyObjRe matches an empty JSON object as a value, e.g. `"locationDetails": {}` — the
// classic "inferred but never filled in" hole.
var emptyObjRe = regexp.MustCompile(`:\s*\{\s*\}`)

// TestEveryBodyOpHasACompleteExample is the completeness gate: every op that declares a JSON
// request body MUST carry a body_example that is real — non-empty, valid JSON, with no empty
// {} object (bare or nested) and not the bare {"id":...} stub. A single-field body like
// {"mdn":"5551234567"} is fine; length is not the test. This fails hard if any body-taking op
// regresses to a stub/empty/missing body, so "comprehensive" is enforced by CI, not by claim.
func TestEveryBodyOpHasACompleteExample(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, op Operation) {
		if !op.TakesBody || op.Multipart {
			return
		}
		ex := strings.TrimSpace(op.BodyExample)
		if ex == "" {
			t.Errorf("%s takes a JSON body but has no body_example", name)
			return
		}
		var v any
		if err := json.Unmarshal([]byte(ex), &v); err != nil {
			t.Errorf("%s body_example is not valid JSON: %v", name, err)
			return
		}
		// A real request body is a non-empty JSON object; null / [] / a scalar all parse as
		// valid JSON but are not a DTO, so reject them explicitly (otherwise the checks below
		// are skipped and the gate silently passes an empty/non-DTO body).
		m, ok := v.(map[string]any)
		if !ok {
			t.Errorf("%s body_example must be a JSON object (a DTO), got %T: %s", name, v, ex)
			return
		}
		if len(m) == 0 || emptyObjRe.MatchString(ex) {
			t.Errorf("%s body_example has an empty {} object (unfilled): %s", name, ex)
		}
		if len(m) == 1 {
			if _, idOnly := m["id"]; idOnly {
				t.Errorf("%s body_example is the bare {\"id\":...} stub, not the real DTO: %s", name, ex)
			}
		}
	}
	for _, en := range d.EntityNames() {
		e, _ := d.Entity(en)
		for _, n := range e.OperationNames() {
			check(en+"."+n, e.Operations[n])
		}
		for _, n := range e.ActionNames() {
			check(en+"."+n+" (action)", e.Actions[n])
		}
	}
}

// dynQueryLiteral matches a Java reflection type string that leaked into a query-param
// name (e.g. "<QueryMap: HashMap<String,String>>", "(@QueryMap dynamic)") — the harvester
// wrote the @QueryMap parameter TYPE instead of the real wire keys, leaving those ops
// uncallable. The real keys were mined from the decompiled callers (workflow wf_173e5408).
var dynQueryLiteral = regexp.MustCompile(`[<>]|QueryMap|HashMap|Map<|dynamic`)

// TestNoDynamicQueryLiterals fails if any op's declared query params still carry a
// reflection literal instead of a real wire key. RED against the pre-fix descriptor (12
// ops carried "<QueryMap: HashMap<String,String>>"-style placeholders).
func TestNoDynamicQueryLiterals(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, op Operation) {
		for _, q := range op.Query {
			if dynQueryLiteral.MatchString(q) {
				t.Errorf("%s has a reflection literal as a query-param name (must be the real wire key): %q", name, q)
			}
		}
	}
	for _, en := range d.EntityNames() {
		e, _ := d.Entity(en)
		for _, n := range e.OperationNames() {
			check(en+"."+n, e.Operations[n])
		}
		for _, n := range e.ActionNames() {
			check(en+"."+n+" (action)", e.Actions[n])
		}
	}
}

func TestDefaultLoads(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatalf("Default() error: %v", err)
	}
	if d.BaseURL != "https://api.prd.vsf.aws.vz-connect.com" {
		t.Errorf("unexpected base_url: %q", d.BaseURL)
	}
	if d.Auth.ClientID == "" {
		t.Error("auth.client_id must not be empty")
	}
	// The whole point of the CLI is broad coverage; guard against an accidentally
	// gutted descriptor.
	if got := len(d.Entities); got < 8 {
		t.Errorf("expected the data model to cover >=8 entities, got %d", got)
	}
	if _, ok := d.Entity("content_filter"); !ok {
		t.Error("expected a content_filter entity")
	}
	// The harvested surface is large; make sure it stayed comprehensive.
	if got := len(d.Entities); got < 40 {
		t.Errorf("expected the harvested data model to cover >=40 entities, got %d", got)
	}
}

func TestParseRejectsEmptyBaseURL(t *testing.T) {
	_, err := Parse([]byte(`{"name":"x","entities":{"a":{}}}`))
	if err == nil {
		t.Fatal("expected an error for empty base_url")
	}
}

func TestParseRejectsNoEntities(t *testing.T) {
	_, err := Parse([]byte(`{"name":"x","base_url":"https://h"}`))
	if err == nil {
		t.Fatal("expected an error for no entities")
	}
}

func TestNamesAreSorted(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	names := d.EntityNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("EntityNames not sorted: %v", names)
		}
	}
}

// The descriptor must preserve every statically-known header name for an op — Codex #20
// flagged that activity_tracking getDailyActivities dropped its required timezone /
// schedule-type headers, leaving the single source of truth incomplete.
func TestDeclaredHeadersPreserved(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := d.Entity("activity_tracking")
	if !ok {
		t.Fatal("no activity_tracking entity")
	}
	op, ok := e.Operations["getDailyActivities"]
	if !ok {
		t.Fatal("no getDailyActivities op")
	}
	want := map[string]bool{"timezone": false, "schedule-type": false, "x-fp-identifier-target-serviceid": false}
	for _, h := range op.Headers {
		if _, ok := want[h]; ok {
			want[h] = true
		}
	}
	for h, seen := range want {
		if !seen {
			t.Errorf("getDailyActivities is missing the declared header %q (have %v)", h, op.Headers)
		}
	}
}

// The catastrophic, irreversible ops must stay marked Destructive so `call`'s --confirm
// guard covers them (Codex flagged pairing.deleteMediaBackupEntries slipping through).
func TestDestructiveOpsMarked(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{"account", "deleteProfile"}, {"account", "deleteMyself"},
		{"account", "selfRemoveProfile"}, {"account", "deleteDevice"},
		{"subscription", "cancelSubscription"}, {"pairing", "unlinkGizmoAccount"},
		{"pairing", "deleteMediaBackupEntries"},
		{"messaging", "deleteMessages"}, {"messaging", "deleteGroupChat"},
		{"messaging", "clearAllGroupChatMessages"}, {"messaging", "deleteGroupMember"},
		{"contacts", "bulkDeleteContacts"},
		{"family_line", "deProvisionFamilyLine"}, {"family_line", "removeUserFromFamilyLine"},
		{"professional_monitoring", "deactivateProfile"},
	}
	for _, w := range want {
		e, ok := d.Entity(w[0])
		if !ok {
			t.Errorf("no entity %q", w[0])
			continue
		}
		op, ok := e.Operations[w[1]]
		if !ok {
			t.Errorf("no op %s.%s", w[0], w[1])
			continue
		}
		if !op.Destructive {
			t.Errorf("%s.%s must be marked Destructive (irreversible)", w[0], w[1])
		}
	}
}

// Every op sharing a wire route (method+path) with a destructive op must ALSO be
// destructive — the backend sees only the route + body, so an unmarked alias would let a
// caller reach the destructive route without --confirm (Codex: professional_monitoring
// reactivateProfile shares deactivateProfile's PATCH route).
func TestDestructiveRoutesFullyGated(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	type route struct{ m, p string }
	byRoute := map[route][]Operation{}
	names := map[route][]string{}
	for en, e := range d.Entities {
		add := func(opn string, op Operation) {
			r := route{op.Method, op.Path}
			byRoute[r] = append(byRoute[r], op)
			names[r] = append(names[r], en+"."+opn)
		}
		for opn, op := range e.Operations {
			add(opn, op)
		}
		for opn, op := range e.Actions {
			add(opn, op)
		}
	}
	for r, ops := range byRoute {
		anyDest := false
		for _, op := range ops {
			anyDest = anyDest || op.Destructive
		}
		if !anyDest {
			continue
		}
		for i, op := range ops {
			if !op.Destructive {
				t.Errorf("%s shares destructive route %s %s but is not gated", names[r][i], r.m, r.p)
			}
		}
	}
}

// Body examples come from each op's own model (not a route-sibling), and multipart ops
// carry no JSON example (Codex #24).
func TestBodyExampleAccuracy(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	if ex := op("schedules", "putScreenTimeData").BodyExample; !strings.Contains(ex, "weeklyLimits") {
		t.Errorf("putScreenTimeData example = %q, want weeklyLimits", ex)
	}
	if ex := op("schedules", "acceptDeclineAskTimeRequest").BodyExample; !strings.Contains(ex, "additionalTimeAction") {
		t.Errorf("acceptDeclineAskTimeRequest example = %q, want the decision field", ex)
	}
	if o := op("messaging", "sendGroupMediaMessage"); o.BodyExample != "" || !o.Multipart {
		t.Errorf("sendGroupMediaMessage must be multipart with no JSON example: multipart=%v ex=%q", o.Multipart, o.BodyExample)
	}
}

// TestLiveVerifiedMutations locks in the mutation bodies verified against the live app via
// eCapture (2026-08-26): the app-block/subcategory POST and the schedules PUT. These assertions
// fail against the pre-verification descriptor (blockApp/updateSubcategory were not confirmed;
// putSchedule carried the incomplete {"id":"12345"} stub) — RED without the capture work.
func TestLiveVerifiedMutations(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	op := func(en, name string) Operation {
		e, _ := d.Entity(en)
		if o, ok := e.Operations[name]; ok {
			return o
		}
		return e.Actions[name]
	}
	// app-block / content subcategory: POST /v8/subcategory, body {"subcategory":{...}}.
	if o := op("app_block", "blockApp"); !o.Confirmed ||
		!strings.Contains(o.BodyExample, `"subcategory"`) || !strings.Contains(o.BodyExample, `"categoryShortName"`) {
		t.Errorf("app_block.blockApp must be confirmed with the subcategory body: confirmed=%v ex=%q", o.Confirmed, o.BodyExample)
	}
	if o := op("content_filter", "updateSubcategory"); !o.Confirmed || !strings.Contains(o.BodyExample, `"subcategory"`) {
		t.Errorf("content_filter.updateSubcategory must be confirmed with a subcategory body: confirmed=%v ex=%q", o.Confirmed, o.BodyExample)
	}
	// schedules PUT: the verified body is a schedules[] array of real fields, not the {"id"} stub.
	put := op("schedules", "putSchedule")
	if !put.Confirmed {
		t.Error("schedules.putSchedule must be confirmed (verified live)")
	}
	for _, want := range []string{`"schedules"`, "scheduleType", "alertOn", "blockContent", "startTime"} {
		if !strings.Contains(put.BodyExample, want) {
			t.Errorf("schedules.putSchedule body_example missing %q: %q", want, put.BodyExample)
		}
	}
	if strings.Contains(put.BodyExample, `"id":"12345"`) {
		t.Errorf("schedules.putSchedule still carries the incomplete stub: %q", put.BodyExample)
	}
	// postSchedule shares the enriched schedules[] schema (create omits scheduleId).
	if ex := op("schedules", "postSchedule").BodyExample; !strings.Contains(ex, "scheduleType") || strings.Contains(ex, `"id":"12345"`) {
		t.Errorf("schedules.postSchedule body_example not enriched: %q", ex)
	}
}
