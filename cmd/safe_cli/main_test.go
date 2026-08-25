package main

import (
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// describe surfaces the declared query parameters so a user can discover that, e.g.,
// accessibility_pin validatePin needs --query pin=... (Codex #20: the discovery flow
// must reveal query params, not hide them in JSON/repo data).
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
	// The FLAGS column tells an agent validatePin takes --query (and needs --service-id).
	if !strings.Contains(s, "query") {
		t.Errorf("describe FLAGS should show query for an op with query params:\n%s", s)
	}
	if !strings.Contains(s, "WHAT IT DOES") {
		t.Errorf("describe should have a WHAT IT DOES (description) column:\n%s", s)
	}
}
