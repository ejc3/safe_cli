package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// syntheticIDs are the only 7–8 digit numbers allowed to appear in any committed .md doc.
// SafePath service/profile/device ids are 7–8 digits; a real captured id (leaked into a doc
// example, as happened in PR #31's README and again via a password literal Codex caught in
// PR #32) must fail this. Kept secret-free: the allowlist is the synthetic example ids, so
// the guard never embeds a real value.
var syntheticIDs = map[string]bool{
	"1000001": true, "2000001": true, "3000001": true, // synthetic GUARDIAN row
	"1000002": true, "2000002": true, "3000002": true, // synthetic DEPENDENT row
}

// TestDocsUseOnlySyntheticIDs scans every committed Markdown doc (not just the README) so
// no doc example can leak per-user account ids. AGENTS.md is a symlink to CLAUDE.md and is
// covered by reading it.
func TestDocsUseOnlySyntheticIDs(t *testing.T) {
	root := filepath.Join("..", "..")
	re := regexp.MustCompile(`\b[0-9]{7,8}\b`)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		// #nosec G122 -- scanning the repo's own committed docs from a fixed relative
		// root; there is no untrusted symlink / TOCTOU surface in a test.
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllString(string(b), -1) {
			if !syntheticIDs[m] {
				t.Errorf("%s contains a non-synthetic %d-digit id %q — docs must use synthetic "+
					"example ids, never captured account values", path, len(m), m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs: %v", err)
	}
}
