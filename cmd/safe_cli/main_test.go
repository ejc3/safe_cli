package main

import (
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// describe surfaces the EXACT declared query-parameter names so a user or agent can construct
// the call straight from `describe` — e.g. accessibility_pin validatePin needs --query pin=...
// and retrievePin needs --query newPin=... (Codex #20 revealed query params exist; Codex #28:
// the generic `query` token is not enough, the names must appear, not hide in JSON/repo data).
func TestDescribeShowsQueryParams(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	rc := &runContext{D: d, G: &Globals{}, Out: &out}
	if err := (&describeCmd{Entity: "accessibility_pin"}).Run(rc); err != nil {
		t.Fatalf("describe: %v", err)
	}
	s := out.String()
	// The FLAGS column must name the exact params: validatePin -> pin, retrievePin -> newPin.
	if !strings.Contains(s, "query=pin") {
		t.Errorf("describe FLAGS should name the exact query param (query=pin) for validatePin:\n%s", s)
	}
	if !strings.Contains(s, "query=newPin") {
		t.Errorf("describe FLAGS should name the exact query param (query=newPin) for retrievePin:\n%s", s)
	}
	if !strings.Contains(s, "WHAT IT DOES") {
		t.Errorf("describe should have a WHAT IT DOES (description) column:\n%s", s)
	}
}

// describe must name the exact --header and --path args too, and must NOT leak the decompiler's
// dynamic-@HeaderMap artifacts as if they were header names (Codex #28).
func TestDescribeShowsHeaderAndPathNames(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	render := func(entity string) string {
		var out strings.Builder
		rc := &runContext{D: d, G: &Globals{}, Out: &out}
		if err := (&describeCmd{Entity: entity}).Run(rc); err != nil {
			t.Fatalf("describe %s: %v", entity, err)
		}
		return out.String()
	}
	// messaging deleteGroupMember has a second placeholder {member-id} beyond the id field.
	if s := render("messaging"); !strings.Contains(s, "path=member-id") {
		t.Errorf("describe should name the extra path placeholder (path=member-id):\n%s", s)
	}
	// account addOnboardingDevice declares the x-pairing-required header; and no row may show a
	// decompiler artifact like "(dynamic @HeaderMap ...)".
	s := render("account")
	if !strings.Contains(s, "header=") || !strings.Contains(s, "x-pairing-required") {
		t.Errorf("describe should name a real declared header (x-pairing-required):\n%s", s)
	}
	if strings.Contains(s, "@HeaderMap") || strings.Contains(s, "dynamic") {
		t.Errorf("describe must not surface decompiler dynamic-header artifacts:\n%s", s)
	}
}

// Confirmed dead-end entities are hidden from `entities` and their ops refused by `call`,
// while `describe` still documents them marked unavailable (disabled, not deleted).
func TestUnavailableDeadEndsHiddenAndRefused(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	// entities --json lists only callable entities: the dead ends are absent (names are
	// quoted, so this is an exact-name check, not a loose substring).
	var ejson strings.Builder
	if err := (&entitiesCmd{}).Run(&runContext{D: d, G: &Globals{JSON: true}, Out: &ejson}); err != nil {
		t.Fatalf("entities --json: %v", err)
	}
	js := ejson.String()
	for _, hidden := range []string{"messaging", "video_calling", "pet_tracker", "tamper", "wearable", "gizmo_activation", "installed_apps"} {
		if strings.Contains(js, `"`+hidden+`"`) {
			t.Errorf("entities --json must omit unavailable entity %q:\n%s", hidden, js)
		}
	}
	if !strings.Contains(js, `"account"`) {
		t.Errorf("entities --json must still list callable entities like account:\n%s", js)
	}
	// The table output must note the hidden entities.
	var eout strings.Builder
	if err := (&entitiesCmd{}).Run(&runContext{D: d, G: &Globals{}, Out: &eout}); err != nil {
		t.Fatalf("entities: %v", err)
	}
	if !strings.Contains(eout.String(), "entities hidden") {
		t.Errorf("entities must note hidden entities:\n%s", eout.String())
	}
	// describe still documents messaging, marked unavailable.
	var dout strings.Builder
	if err := (&describeCmd{Entity: "messaging"}).Run(&runContext{D: d, G: &Globals{}, Out: &dout}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !strings.Contains(dout.String(), "UNAVAILABLE") {
		t.Errorf("describe must mark unavailable ops:\n%s", dout.String())
	}
	// call refuses an unavailable op up front (before touching tokens), citing the reason.
	err = (&callCmd{Entity: "messaging", Op: "createNewGroup"}).Run(&runContext{D: d, G: &Globals{}, Out: &strings.Builder{}})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("call must refuse an unavailable op; got %v", err)
	}
}
