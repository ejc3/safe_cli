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

// TestDocsUseOnlySyntheticIDs scans every committed Markdown doc AND the HTML home page so
// no doc example can leak per-user account ids. AGENTS.md is a symlink to CLAUDE.md and is
// covered by reading it; docs/index.html is covered because it is served publicly.
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
		if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".html") {
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

// TestImportDocUsesPhoneKey pins docs/PROCESS.md's `auth import` TokenSet example to the
// current on-disk key. A persisted TokenSet serializes the phone number under "phone"
// (see internal/tokenstore), so the hand-built import example must teach "phone", not the
// deprecated "mdn". Wire request bodies elsewhere in the doc keep "mdn" (the API field);
// this guards only the TokenSet-shaped example, identified by its `tokens":[{"id_token`
// opener so the OTP bodies are not matched.
func TestImportDocUsesPhoneKey(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "PROCESS.md"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, `tokens":[{"id_token`) {
			continue
		}
		found = true
		if !strings.Contains(line, `"phone"`) {
			t.Errorf("PROCESS.md auth-import TokenSet example must use \"phone\": %s", strings.TrimSpace(line))
		}
		if strings.Contains(line, `"mdn"`) {
			t.Errorf("PROCESS.md auth-import TokenSet example must not teach the deprecated \"mdn\" key: %s", strings.TrimSpace(line))
		}
	}
	if !found {
		t.Fatal("did not find the TokenSet import example in PROCESS.md (tokens\":[{\"id_token …) — did the doc move?")
	}
}
