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
	if _, ok := d.Entity("web_filter"); !ok {
		t.Error("expected a web_filter entity")
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
