package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test's source file to the directory holding go.mod,
// so the workflow assertions below do not depend on the test's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(self)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from " + self)
		}
		dir = parent
	}
}

// nonCommentLines returns the file's lines with whole-line YAML comments removed, so
// a trigger that is only *mentioned* in a comment cannot satisfy the assertions below.
func nonCommentLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "#") {
			continue
		}
		out = append(out, ln)
	}
	return out
}

// TestReviewGateReevaluatesOnReviewEvents pins the contract that closes the codex P1 on
// PR #104: the review-thread gate must be re-evaluated when review state changes, not only
// on push. A gate wired to the default `pull_request` activities alone (opened/synchronize/
// reopened) leaves its last green check attached to the head SHA when a finding is added
// after the last push — GitHub's native conversation rule clears and the PR merges with an
// undisposed finding. So the workflow that runs the gate script MUST also trigger on
// pull_request_review and pull_request_review_comment. This test fails RED without those
// triggers (the original design ran the job in ci.yml on pull_request only).
func TestReviewGateReevaluatesOnReviewEvents(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".github", "workflows", "review-gate.yml")
	lines := nonCommentLines(t, path)

	// The workflow must actually run the enforcement script; a trigger set means nothing
	// if it does not invoke the gate.
	runsGate := false
	for _, ln := range lines {
		if strings.Contains(ln, "check-review-threads.sh") {
			runsGate = true
			break
		}
	}
	if !runsGate {
		t.Errorf("%s does not run check-review-threads.sh; the gate is not enforced", path)
	}

	// Each trigger must appear as a top-level key under `on:` (two-space indent), not merely
	// as a substring somewhere in the file.
	for _, trigger := range []string{"pull_request", "pull_request_review", "pull_request_review_comment"} {
		re := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(trigger) + `:`)
		if !re.MatchString(strings.Join(lines, "\n")) {
			t.Errorf("%s missing `on:` trigger %q — the gate would not re-evaluate when review state changes", path, trigger)
		}
	}

	joined := strings.Join(lines, "\n")

	// cancel-in-progress must be false so a superseded re-evaluation is not cancelled — a
	// cancelled run shows as a grey X, which reads like a real failure on an informational check.
	if regexp.MustCompile(`(?m)cancel-in-progress:\s*true`).MatchString(joined) {
		t.Errorf("%s sets cancel-in-progress: true — a cancelled run reads as a failure; it must be false", path)
	}
	if !regexp.MustCompile(`(?m)cancel-in-progress:\s*false`).MatchString(joined) {
		t.Errorf("%s must set cancel-in-progress: false so review-event re-evaluations queue instead of cancelling", path)
	}
}
