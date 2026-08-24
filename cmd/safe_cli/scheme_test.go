package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppletSourceEmbedsPathAndHandler(t *testing.T) {
	src := appletSource("/Users/x/Library/Application Support/safe_cli/vsfapp_redirect")
	for _, want := range []string{"on open location", "this_URL", "safe_cli/vsfapp_redirect"} {
		if !strings.Contains(src, want) {
			t.Errorf("applet source missing %q:\n%s", want, src)
		}
	}
}

// The embedded path's quote and backslash must be escaped so the AppleScript string
// literal stays valid.
func TestAppletSourceEscapes(t *testing.T) {
	src := appletSource(`/tmp/a"b\c/vsfapp_redirect`)
	if !strings.Contains(src, `a\"b\\c`) {
		t.Errorf("path not escaped for AppleScript:\n%s", src)
	}
}

func TestPlistBuddyCommandsDeclareScheme(t *testing.T) {
	joined := strings.Join(plistBuddyCommands(), "\n")
	for _, want := range []string{"CFBundleURLSchemes", "string " + vsfappScheme, "LSUIElement", handlerBundleID} {
		if !strings.Contains(joined, want) {
			t.Errorf("plist commands missing %q", want)
		}
	}
}

func TestWaitForRedirectReadsFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "redir")
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = os.WriteFile(f, []byte("  vsfapp://com.verizon.familybase.parent/signin?code=X\n"), 0o600)
	}()
	got, err := waitForRedirect(context.Background(), f, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForRedirect: %v", err)
	}
	if got != "vsfapp://com.verizon.familybase.parent/signin?code=X" {
		t.Errorf("got %q, want the trimmed URL", got)
	}
}

func TestWaitForRedirectContextCancel(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := waitForRedirect(ctx, filepath.Join(dir, "never"), 5*time.Millisecond); err == nil {
		t.Error("want an error when the file never appears and ctx is done")
	}
}

// An empty file must not be mistaken for a captured redirect.
func TestWaitForRedirectIgnoresEmptyFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "redir")
	if err := os.WriteFile(f, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := waitForRedirect(ctx, f, 5*time.Millisecond); err == nil {
		t.Error("an empty/whitespace file must not count as a capture")
	}
}

func TestVsfappRedirectPath(t *testing.T) {
	p, err := vsfappRedirectPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, filepath.Join("safe_cli", redirectFileName)) {
		t.Errorf("redirect path = %q", p)
	}
}

// The platform guard: off macOS, registration returns a clear error rather than shelling
// out to tools that do not exist.
func TestRegisterSchemeGuardsNonDarwin(t *testing.T) {
	orig := schemeGOOS
	schemeGOOS = "linux"
	defer func() { schemeGOOS = orig }()
	if _, _, err := registerScheme(&strings.Builder{}); err == nil {
		t.Error("want a macOS-only error off darwin")
	}
}
