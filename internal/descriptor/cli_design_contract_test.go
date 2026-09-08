package descriptor

import (
	"os"
	"strings"
	"testing"
)

// TestCLIDesignPinsSchemaContracts pins three contracts of the proposed per-op `cli`
// descriptor block in docs/CLI-DESIGN.md that Codex flagged on PR #66 as defects in the
// spec. No runtime test can cover them until the block exists in code, so this
// source-level assertion holds the contract in the design (the same way
// scenarios_coverage_test.go pins docs/agent-scenarios.md) until the implementation's
// descriptor tests take over. Each clause, if lost, would let the generator send a wrong
// request:
//  1. conditional omission — a `$var?` property is dropped, so an indefinite pause OMITS
//     pauseSchedule as the wire-verified body requires;
//  2. explicit query constants — the name-only query[] cannot carry categorySupported=v6
//     or strategy=NotNull, so a `query` map must;
//  3. unclassified body-example fields are REJECTED — a create must not ship the sample
//     server-assigned geofenceId, nor sendInvite's stray pet/contact/Wi-Fi fields.
func TestCLIDesignPinsSchemaContracts(t *testing.T) {
	b, err := os.ReadFile("../../docs/CLI-DESIGN.md")
	if err != nil {
		t.Fatalf("read design doc: %v", err)
	}
	doc := string(b)
	checks := []struct {
		clause  string
		needles []string
	}{
		{"conditional omission ($var?)", []string{"$var?", "omitted from the body entirely"}},
		{"explicit query constants", []string{"`query`: an explicit map", "{\"categorySupported\": \"v6\"}"}},
		{"reject unclassified fields", []string{"**rejected** by the descriptor test", "sample `geofenceId` must be dropped"}},
	}
	for _, c := range checks {
		for _, n := range c.needles {
			if !strings.Contains(doc, n) {
				t.Errorf("%s: design doc no longer states the contract (missing %q)", c.clause, n)
			}
		}
	}
}
