package descriptor

import (
	"strings"
	"testing"
)

// From the navigation test: getTopApps 400s without `date` and getThreats 400s without
// `eventId`, then works. Marking those required makes `describe` show them with * so an
// agent supplies them instead of guessing (issue-driven nav fix).
func TestReportOpsMarkRequiredQuery(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ ent, op, req string }{
		{"most_used_apps", "getTopApps", "date"},
		{"security_threat", "getThreats", "eventId"},
	} {
		op := d.Entities[c.ent].Operations[c.op]
		found := false
		for _, r := range op.RequiredQuery {
			if r == c.req {
				found = true
			}
		}
		if !found {
			t.Errorf("%s.%s must mark %q required_query (backend 400s without it); got %v", c.ent, c.op, c.req, op.RequiredQuery)
		}
	}
}

// getProvisioningStatus and pubnub.getPubNubConfig are account-scoped: they answer only to
// the account holder's (guardian's) service id, and 403 for a child's. The description must
// say so, or an agent targets a child and gets a bare 403 (nav test: family_line scenarios).
func TestAccountScopedOpsDocumentGuardianTarget(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ ent, op string }{
		{"family_line", "getProvisioningStatus"},
		{"pubnub", "getPubNubConfig"},
	} {
		desc := d.Entities[c.ent].Operations[c.op].Description
		low := strings.ToLower(desc)
		if !strings.Contains(low, "guardian") && !strings.Contains(low, "account holder") {
			t.Errorf("%s.%s description must say it is account-scoped (target the guardian's service id):\n%s", c.ent, c.op, desc)
		}
	}
}
