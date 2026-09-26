package main

import (
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
)

// TestReadVerbsRenderAsTables pins that the noisy generated read verbs the UX audit (#90)
// flagged now declare an output.table so a parent gets a scannable table, not raw JSON.
func TestReadVerbsRenderAsTables(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{ // "area verb" -> required table columns
		"apps list":   {"name", "enabledCount", "totalCount"},
		"filter show": {"name", "enabledCount"},
	}
	seen := map[string]bool{}
	for _, en := range d.EntityNames() {
		e, _ := d.Entity(en)
		for _, on := range e.OperationNames() {
			for _, c := range e.Operations[on].CLI {
				key := c.Area + " " + c.Verb
				cols, ok := want[key]
				if !ok {
					continue
				}
				seen[key] = true
				if c.Output == nil || len(c.Output.Table) == 0 {
					t.Errorf("%s should declare output.table", key)
					continue
				}
				have := map[string]bool{}
				for _, col := range c.Output.Table {
					path, _, _ := strings.Cut(col, ":")
					have[path] = true
				}
				for _, col := range cols {
					if !have[col] {
						t.Errorf("%s output.table missing %q; got %v", key, col, c.Output.Table)
					}
				}
			}
		}
	}
	for key := range want {
		if !seen[key] {
			t.Errorf("verb %q not found in the descriptor", key)
		}
	}
}

// TestOutputTableAliasedHeaders: a "path:HEADER" column renders HEADER (uppercased) as the
// title while digging the value from path — so nested paths get readable headers.
func TestOutputTableAliasedHeaders(t *testing.T) {
	resp := &client.Response{Status: 200, Body: []byte(`[{"name":"Social Media","enabledCount":2,"totalCount":15}]`)}
	c := &descriptor.CLI{Output: &descriptor.Output{Table: []string{"name:APP GROUP", "enabledCount:BLOCKED", "totalCount:TOTAL"}}}
	var out strings.Builder
	if err := writeVerbResponse(&out, false, resp, c, nil); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"APP GROUP", "BLOCKED", "TOTAL", "Social Media", "15"} {
		if !strings.Contains(s, want) {
			t.Errorf("table output missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "ENABLEDCOUNT") || strings.Contains(s, "TOTALCOUNT") {
		t.Errorf("raw field-path headers leaked (aliases ignored):\n%s", s)
	}
}
