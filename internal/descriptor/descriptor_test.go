package descriptor

import (
	"strings"
	"testing"
)

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
