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
