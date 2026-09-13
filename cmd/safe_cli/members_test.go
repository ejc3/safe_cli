package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// parseAccount must flatten the nested account-details shape (accounts[].userprofiles[].
// services[]) into one addressable row per service, with the parent-facing role label
// (child|guardian, plus is_child), the pairing an agent keys off, and the stated order
// (guardian, then PAIRED child, then UNPAIRED). Synthetic values only — no real account data.
func TestParseAccountMembers(t *testing.T) {
	body := []byte(`{"accounts":[{"accountId":111,"familyName":"Test","userprofiles":[
		{"userProfileId":202,"profileName":"Kid","services":[
			{"serviceId":9002,"userProfileId":202,"roleName":"DEPENDENT","deviceId":8002,"pairingStatus":"UNPAIRED","planName":"Family Plus"}]},
		{"userProfileId":201,"profileName":"Parent","services":[
			{"serviceId":9001,"userProfileId":201,"roleName":"GUARDIAN","deviceId":8001,"pairingStatus":"PAIRED","planName":"Family Plus"}]},
		{"userProfileId":203,"profileName":"NoService","services":[]}
	]}]}`)
	a, err := parseAccount(body)
	if err != nil {
		t.Fatalf("parseAccount: %v", err)
	}
	if a.ID != 111 {
		t.Errorf("account id = %d, want 111", a.ID)
	}
	if len(a.Members) != 2 {
		t.Fatalf("got %d members, want 2 (the service-less profile yields no row)", len(a.Members))
	}
	// Guardian sorts first even though the API listed the child first.
	parent, kid := a.Members[0], a.Members[1]
	if parent.Role != "guardian" || parent.IsChild || parent.ServiceID != 9001 {
		t.Errorf("guardian row = %+v", parent)
	}
	if kid.Name != "Kid" || kid.Role != "child" || !kid.IsChild || kid.ServiceID != 9002 ||
		kid.ProfileID != 202 || kid.DeviceID != 8002 || kid.Pairing != "UNPAIRED" {
		t.Errorf("child row = %+v", kid)
	}
}

// A profile whose service omits its own userProfileId falls back to the profile's id, so
// the ProfileID column is always populated.
func TestParseAccountProfileIDFallback(t *testing.T) {
	body := []byte(`{"accounts":[{"userprofiles":[
		{"userProfileId":777,"profileName":"K","services":[{"serviceId":9,"roleName":"DEPENDENT"}]}]}]}`)
	a, err := parseAccount(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Members) != 1 || a.Members[0].ProfileID != 777 {
		t.Errorf("want ProfileID fallback to 777, got %+v", a.Members)
	}
}

// --find is a case-insensitive substring on the name; --role accepts child|guardian and
// dependent as an alias for child; both empty returns everything.
func TestFilterMembers(t *testing.T) {
	ms := []member{
		{Name: "You", Role: "guardian"},
		{Name: "Alex", Role: "child", IsChild: true},
		{Name: "Alexandra", Role: "child", IsChild: true},
	}
	if got := filterMembers(ms, "", ""); len(got) != 3 {
		t.Errorf("no filter: %d rows", len(got))
	}
	if got := filterMembers(ms, "alex", ""); len(got) != 2 {
		t.Errorf("--find alex: %d rows, want 2 (substring, case-insensitive)", len(got))
	}
	if got := filterMembers(ms, "", "guardian"); len(got) != 1 || got[0].Name != "You" {
		t.Errorf("--role guardian: %+v", got)
	}
	if got := filterMembers(ms, "", "dependent"); len(got) != 2 {
		t.Errorf("--role dependent (alias of child): %d rows", len(got))
	}
	if got := filterMembers(ms, "andra", "child"); len(got) != 1 || got[0].Name != "Alexandra" {
		t.Errorf("--find + --role: %+v", got)
	}
	// No match must be an EMPTY array, not nil: `members --json --find zzz` prints [] so a
	// collection-shaped response never changes type on a normal no-results search.
	if got := filterMembers(ms, "zzz", ""); got == nil || len(got) != 0 {
		t.Errorf("no-match filter must return a non-nil empty slice, got %#v", got)
	}
}

// The role label never leaks the API's wire values, and an unknown role still shows.
func TestRoleLabel(t *testing.T) {
	for in, want := range map[string]string{"DEPENDENT": "child", "GUARDIAN": "guardian", "guardian": "guardian", "OBSERVER": "observer"} {
		if got := roleLabel(in); got != want {
			t.Errorf("roleLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if !strings.Contains(membersOrder, "PAIRED before UNPAIRED") {
		t.Errorf("membersOrder must state the pairing order: %q", membersOrder)
	}
}

// An account with no service rows still lists as an empty ARRAY: `members --json` must print
// [] (a stable collection type), not null (Codex #68 round 2).
func TestParseAccountNoServicesIsEmptyArray(t *testing.T) {
	a, err := parseAccount([]byte(`{"accounts":[{"accountId":1,"userprofiles":[{"userProfileId":2,"profileName":"P","services":[]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.Members == nil || len(a.Members) != 0 {
		t.Errorf("want a non-nil empty member list, got %#v", a.Members)
	}
	if got := filterMembers(a.Members, "", ""); got == nil {
		t.Error("the unfiltered listing must stay a non-nil slice")
	}
}

// `members --help` itself states the row order (docs/CLI-DESIGN.md §5), not only the
// post-request footer, so "the first child" is well-defined before any call (Codex #68).
func TestMembersHelpStatesOrder(t *testing.T) {
	var buf bytes.Buffer
	var cli CLI
	parser, err := kong.New(&cli, kong.Name("safe_cli"), kong.Exit(func(int) {}), kong.Writers(&buf, &buf))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = parser.Parse([]string{"members", "--help"})
	out := buf.String()
	if !strings.Contains(out, "PAIRED before UNPAIRED") || !strings.Contains(out, "--child") {
		t.Errorf("members --help must state the order and the --child target:\n%s", out)
	}
}

// The row an agent reads carries a stable `paired` boolean next to the wire-level pairing
// string (docs/CLI-DESIGN.md §2), and the text footer points at --child, the verbs' target
// (--service-id is `call`'s) (Codex #68 round 5).
func TestMemberPairedBoolAndFooterTarget(t *testing.T) {
	a, err := parseAccount([]byte(`{"accounts":[{"accountId":1,"userprofiles":[
		{"userProfileId":2,"profileName":"K","services":[{"serviceId":9,"roleName":"DEPENDENT","pairingStatus":"UNPAIRED"}]},
		{"userProfileId":3,"profileName":"P","services":[{"serviceId":8,"roleName":"GUARDIAN","pairingStatus":"PAIRED"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !a.Members[0].Paired || a.Members[1].Paired {
		t.Errorf("paired must mirror PAIRED: %+v", a.Members)
	}
	if !strings.Contains(membersFooter, "--child") || !strings.Contains(membersFooter, "call") {
		t.Errorf("the footer must direct to --child and reserve --service-id for call: %q", membersFooter)
	}
}
