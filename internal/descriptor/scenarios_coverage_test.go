package descriptor

import (
	"os"
	"strings"
	"testing"
)

// TestAgentScenariosCoverEveryAvailableOp guards docs/agent-scenarios.md: every
// available (callable) operation in the descriptor must be referenced by at least
// one scenario as `entity.op`, so the blind-agent test suite stays at 100% coverage
// as the descriptor grows. Product-unavailable ops are exempt (they only appear as
// "agent should detect it's unavailable" cases).
func TestAgentScenariosCoverEveryAvailableOp(t *testing.T) {
	b, err := os.ReadFile("../../docs/agent-scenarios.md")
	if err != nil {
		t.Fatalf("read scenarios file: %v", err)
	}
	text := string(b)
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, ename := range d.EntityNames() {
		e, _ := d.Entity(ename)
		check := func(names []string, ops map[string]Operation) {
			for _, oname := range names {
				if !ops[oname].Available() {
					continue
				}
				if !strings.Contains(text, ename+"."+oname) {
					missing = append(missing, ename+"."+oname)
				}
			}
		}
		check(e.OperationNames(), e.Operations)
		check(e.ActionNames(), e.Actions)
	}
	if len(missing) > 0 {
		t.Errorf("%d available ops missing from docs/agent-scenarios.md: %v", len(missing), missing)
	}
}
