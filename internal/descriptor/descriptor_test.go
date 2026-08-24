package descriptor

import "testing"

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
