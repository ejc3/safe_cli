package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// helpFor renders kong's help for the given args (e.g. "pause-internet", "pause", "--help")
// without running any command: kong's exit is replaced by a no-op and its writers captured.
func helpFor(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("safe_cli"),
		kong.Exit(func(int) {}),
		kong.Writers(&buf, &buf),
	)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, _ = parser.Parse(args) // --help prints and "exits" via the no-op
	return buf.String()
}

// TestAuthLoginPhoneFlag pins the user-facing noun: the line-verification flag is
// --phone (nobody recognises "mdn"). The old --mdn spelling must be gone from help.
func TestAuthLoginPhoneFlag(t *testing.T) {
	h := helpFor(t, "auth", "login", "--help")
	if !strings.Contains(h, "--phone") {
		t.Errorf("auth login help must offer --phone; got:\n%s", h)
	}
	if strings.Contains(h, "--mdn") || strings.Contains(h, "MDN") {
		t.Errorf("auth login help must not mention mdn/MDN; got:\n%s", h)
	}
}
