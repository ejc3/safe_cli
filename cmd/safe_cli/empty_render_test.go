package main

import (
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
)

// TestEmptyMessageForTablelessVerb: a table-less verb that opted in with empty_message renders
// a data-empty body (every leaf null/0/false/"") as that one line — the fix for `calls log`
// dumping a blob of nulls — while a populated body prints normally and --json stays raw.
func TestEmptyMessageForTablelessVerb(t *testing.T) {
	c := &descriptor.CLI{Output: &descriptor.Output{EmptyMessage: "No call or text activity in that range."}}
	empty := &client.Response{Status: 200, Body: []byte(`{"totalCalls":0,"totalTexts":0,"callActivity":null,"textActivity":null}`)}

	var out strings.Builder
	if err := writeVerbResponse(&out, false, empty, c, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "No call or text activity in that range." {
		t.Errorf("empty tableless body should render the empty_message line, got:\n%s", out.String())
	}

	// A populated body must NOT be swallowed by the empty line.
	populated := &client.Response{Status: 200, Body: []byte(`{"totalCalls":2,"callActivity":[{"otherParty":"5551212"}]}`)}
	out.Reset()
	if err := writeVerbResponse(&out, false, populated, c, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "No call or text activity") || !strings.Contains(out.String(), "otherParty") {
		t.Errorf("populated body must print its data, not the empty line:\n%s", out.String())
	}

	// --json keeps the raw structure so an agent still sees the zeros/nulls.
	out.Reset()
	if err := writeVerbResponse(&out, true, empty, c, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "No call or text activity") || !strings.Contains(out.String(), "totalCalls") {
		t.Errorf("--json must stay raw, not render the empty_message line:\n%s", out.String())
	}
}

// TestEmptyTableRendersNoRecords: a table verb whose body carries no rows prints a plain
// "No records found." (this is what `location where --member <no match>` now shows) rather
// than falling through to a blob; a body with rows still renders the table.
func TestEmptyTableRendersNoRecords(t *testing.T) {
	c := &descriptor.CLI{Output: &descriptor.Output{Table: []string{"memberName:WHO", "isLastKnownLoc:LAST-KNOWN"}}}

	var out strings.Builder
	if err := writeVerbResponse(&out, false, &client.Response{Status: 200, Body: []byte(`[]`)}, c, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "No records found." {
		t.Errorf("an empty listing should say 'No records found.', got:\n%s", out.String())
	}

	out.Reset()
	if err := writeVerbResponse(&out, false, &client.Response{Status: 200, Body: []byte(`[{"memberName":"Alex","isLastKnownLoc":true}]`)}, c, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Alex") || strings.Contains(out.String(), "No records found") {
		t.Errorf("a non-empty listing must render its table:\n%s", out.String())
	}
}

// TestLocationWhereMemberFilter pins the fix for the older UX finding: `location where` takes a
// --member name filter (find:memberName, so it is a case-insensitive substring on the enriched
// name), and `calls log` carries the empty_message that TestEmptyMessageForTablelessVerb relies on.
func TestLocationWhereMemberFilter(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	find := func(entity, op, wantArea, wantVerb string) *descriptor.CLI {
		e, _ := d.Entity(entity)
		for _, c := range e.Operations[op].CLI {
			if c.Area == wantArea && c.Verb == wantVerb {
				return c
			}
		}
		t.Fatalf("verb %s %s (op %s.%s) not found", wantArea, wantVerb, entity, op)
		return nil
	}

	where := find("location", "getDashboardDetails", "location", "where")
	var member *descriptor.Flag
	for i := range where.Flags {
		if where.Flags[i].Name == "member" {
			member = &where.Flags[i]
		}
	}
	if member == nil {
		t.Fatal("location where must declare a --member flag")
	}
	if member.MapsTo != "find:memberName" {
		t.Errorf("--member must map to find:memberName (a substring filter on the enriched name), got %q", member.MapsTo)
	}

	log := find("calls_and_texts", "getCallAndTextActivityListV7", "calls", "log")
	if log.Output == nil || log.Output.EmptyMessage == "" {
		t.Error("calls log must carry an empty_message so an empty range does not print a blob of nulls")
	}
}

// TestEnrichmentFilterConflict: when name enrichment did not run (account lookup failed) but a
// --member (find:memberName) selector was requested, applying it would prune every record and
// misreport a lookup failure as "no matches" — so it must raise an error instead. When
// enrichment ran, or no name selector was given, there is no conflict.
func TestEnrichmentFilterConflict(t *testing.T) {
	byName := map[string]string{"memberName": "connor"}
	// Enrichment failed + a name find requested -> error.
	if err := enrichmentFilterConflict(false, byName, nil); err == nil {
		t.Error("a name find over an unenriched body must error, not silently prune to empty")
	}
	// Same via a filter: destination.
	if err := enrichmentFilterConflict(false, nil, map[string]any{"memberName": "connor"}); err == nil {
		t.Error("a name filter over an unenriched body must error")
	}
	// Enrichment succeeded -> no conflict even with a name selector.
	if err := enrichmentFilterConflict(true, byName, nil); err != nil {
		t.Errorf("no conflict when enrichment ran: %v", err)
	}
	// Enrichment failed but the selector is on some OTHER field -> no conflict.
	if err := enrichmentFilterConflict(false, map[string]string{"city": "san jose"}, nil); err != nil {
		t.Errorf("a non-name selector does not depend on enrichment: %v", err)
	}
}
