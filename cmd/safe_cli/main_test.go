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
	if !strings.Contains(s, "QUERY") {
		t.Errorf("describe table is missing the QUERY column:\n%s", s)
	}
	// validatePin declares the `pin` query param; it must appear in the human table.
	if !strings.Contains(s, "pin") {
		t.Errorf("validatePin's declared query param `pin` is not shown:\n%s", s)
	}
}
