package main

import (
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// TestClosestSuggestsNearbyEntity: a mistyped entity name offers the real one first (nav/UX
// audit #90: `describe locations` should hint `location`). A far-off name yields nothing
// rather than a misleading guess.
func TestClosestSuggestsNearbyEntity(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	names := d.EntityNames()

	got := closest("locations", names, 3)
	if len(got) == 0 || got[0] != "location" {
		t.Errorf("closest(\"locations\") should lead with \"location\", got %v", got)
	}
	// a plausible near-miss for a report entity
	if got := closest("security_threats", names, 3); len(got) == 0 || got[0] != "security_threat" {
		t.Errorf("closest(\"security_threats\") should lead with \"security_threat\", got %v", got)
	}
	// gibberish -> no suggestion (don't fabricate one)
	if got := closest("zzzzzzzzzz", names, 3); len(got) != 0 {
		t.Errorf("closest(gibberish) should be empty, got %v", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"location", "location", 0},
		{"locations", "location", 1},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestDescribeUnknownEntitySuggests: the describe error offers a did-you-mean, matching the
// top-level command behavior (UX audit #90 papercut).
func TestDescribeUnknownEntitySuggests(t *testing.T) {
	d, _ := descriptor.Default()
	rc := &runContext{D: d, G: &Globals{}, Out: &strings.Builder{}}
	err := (&describeCmd{Entity: "locations"}).Run(rc)
	if err == nil || !strings.Contains(err.Error(), "location") {
		t.Errorf("describe locations should suggest \"location\", got: %v", err)
	}
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "did you mean") {
		t.Errorf("describe error should say \"did you mean\", got: %v", err)
	}
}

// TestClosestRejectsWeakMatch: a short name whose nearest entity is 3 edits away (member ->
// tamper) gets NO suggestion at the tightened threshold — the audit flagged the nonsense
// "did you mean tamper?".
func TestClosestRejectsWeakMatch(t *testing.T) {
	d, _ := descriptor.Default()
	names := d.EntityNames()
	if got := closest("member", names, 2); len(got) != 0 {
		t.Errorf("`member` should get no suggestion at maxDist 2, got %v", got)
	}
	if got := closest("locations", names, 2); len(got) == 0 || got[0] != "location" {
		t.Errorf("`locations` should still suggest `location` at maxDist 2, got %v", got)
	}
}

func TestFirstCommandWord(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"call"}, "call"},
		{[]string{"describe"}, "describe"},
		{[]string{"--json", "members"}, "members"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := firstCommandWord(c.args); got != c.want {
			t.Errorf("firstCommandWord(%v)=%q want %q", c.args, got, c.want)
		}
	}
}
