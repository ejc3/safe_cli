package main

import (
	"testing"

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
					have[col] = true
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
