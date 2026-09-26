package main

import (
	jsonpkg "encoding/json"
	fmtpkg "fmt"
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
		"apps list":       {"group", "name", "id", "enabled"},
		"filter show":     {"name", "enabledCount"},
		"calls schedules": {"name", "scheduleType", "startTime", "endTime"},
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

// TestFlattenField expands nested children into flat rows, tagging each with a renamed
// parent field — the mechanism that puts app ids back in `apps list` (audit round 2).
func TestFlattenField(t *testing.T) {
	body := []byte(`{"Apps & websites":[{"name":"Social Media","subCategories":[{"name":"TikTok","id":10037,"enabled":false}]}]}`)
	got := flattenField(body, "subCategories", map[string]string{"name": "group"})
	var rows []map[string]any
	if err := jsonUnmarshal(got, &rows); err != nil {
		t.Fatalf("bad json: %v (%s)", err, got)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 flattened row, got %d", len(rows))
	}
	if rows[0]["name"] != "TikTok" || rows[0]["group"] != "Social Media" {
		t.Errorf("app row lost id/group: %v", rows[0])
	}
	if fmtSprint(rows[0]["id"]) != "10037" {
		t.Errorf("app id lost: %v", rows[0]["id"])
	}
}

func jsonUnmarshal(b []byte, v any) error { return jsonpkg.Unmarshal(b, v) }
func fmtSprint(v any) string              { return fmtpkg.Sprint(v) }
