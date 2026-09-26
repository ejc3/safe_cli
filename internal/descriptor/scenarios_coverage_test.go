package descriptor

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// scenarioVerbRe pulls a backtick-delimited `entity.op` token out of a "Verbs exercised:"
// line. The op part matches everything up to the closing backtick, because some op names
// carry a parenthetical (config.getConfigData (Call variant), todo.invoke (getTodos)) — the
// whole thing is one token and must be captured, and compared, in full.
var scenarioVerbRe = regexp.MustCompile("`([a-zA-Z0-9_]+\\.[^`]+)`")

// TestAgentScenariosCoverEveryAvailableOp guards docs/agent-scenarios.md: every
// available (callable) operation in the descriptor must be declared by at least one
// scenario's "Verbs exercised:" line as `entity.op`, so the blind-agent test suite
// stays at 100% coverage as the descriptor grows. Product-unavailable ops are exempt
// (they only appear as "agent should detect it's unavailable" cases).
//
// Coverage is checked against the EXACT tokens on the "Verbs exercised:" lines, not a
// substring search of the whole file. That matters twice: an op named only in a
// scenario's prose (never in a Verbs-exercised list) must not count as covered, and a
// short op name must not be satisfied by a longer one that contains it — website.postWebsite
// is not covered by website.postWebsites the way strings.Contains once treated it.
func TestAgentScenariosCoverEveryAvailableOp(t *testing.T) {
	b, err := os.ReadFile("../../docs/agent-scenarios.md")
	if err != nil {
		t.Fatalf("read scenarios file: %v", err)
	}
	exercised := map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "Verbs exercised:") {
			continue
		}
		for _, m := range scenarioVerbRe.FindAllStringSubmatch(line, -1) {
			exercised[m[1]] = true
		}
	}
	if len(exercised) == 0 {
		t.Fatal("no `entity.op` tokens found on any \"Verbs exercised:\" line — the doc format changed")
	}
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
				if !exercised[ename+"."+oname] {
					missing = append(missing, ename+"."+oname)
				}
			}
		}
		check(e.OperationNames(), e.Operations)
		check(e.ActionNames(), e.Actions)
	}
	if len(missing) > 0 {
		t.Errorf("%d available ops missing from a \"Verbs exercised:\" line in docs/agent-scenarios.md: %v", len(missing), missing)
	}
}
