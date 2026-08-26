package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestReadmeUsesOnlySyntheticIDs guards the public README against committing per-user
// account values. SafePath service/profile/device ids are 7–8 digit numbers; every such
// number in the README must be one of the documented synthetic example ids. A real captured
// id (as leaked into the members example in PR #31) fails this — see AGENTS.md's prohibition
// on committing per-user values. Kept secret-free: the allowlist is the synthetic ids, so
// the guard never embeds a real one.
func TestReadmeUsesOnlySyntheticIDs(t *testing.T) {
	path := filepath.Join("..", "..", "README.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("README not found at %s: %v", path, err)
	}
	allow := map[string]bool{
		"1000001": true, "2000001": true, "3000001": true, // synthetic GUARDIAN row
		"1000002": true, "2000002": true, "3000002": true, // synthetic DEPENDENT row
	}
	for _, m := range regexp.MustCompile(`\b[0-9]{7,8}\b`).FindAllString(string(b), -1) {
		if !allow[m] {
			t.Errorf("README contains a non-synthetic %d-digit id %q — replace account-derived "+
				"service/profile/device ids with synthetic example values", len(m), m)
		}
	}
}
