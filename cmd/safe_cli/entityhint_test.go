package main

import (
	"errors"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// A bare entity name used as a command must be answered with a hint pointing to call/describe,
// not the raw kong "unexpected argument" error (issue #83).
func TestEntityCommandHint(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	// invite is an entity, not a command; kong rejects it.
	h := entityCommandHint(d, []string{"invite", "replaceDevice"}, errors.New("unexpected argument invite"))
	if h == "" {
		t.Fatal("expected a hint for a bare entity name used as a command")
	}
	for _, want := range []string{"call invite", "describe invite", "entity"} {
		if !contains(h, want) {
			t.Errorf("hint should mention %q; got:\n%s", want, h)
		}
	}
	// A real command that fails for another reason gets no entity hint.
	if h := entityCommandHint(d, []string{"pause-internet", "pause"}, errors.New("missing flags: --child")); h != "" {
		t.Errorf("no entity hint for a real command; got: %s", h)
	}
	// A successful parse (nil error) yields no hint.
	if h := entityCommandHint(d, []string{"members"}, nil); h != "" {
		t.Errorf("no hint on success; got: %s", h)
	}
	// A non-entity unknown token gets no entity hint (kong's own error stands).
	if h := entityCommandHint(d, []string{"bogus"}, errors.New("unexpected argument bogus")); h != "" {
		t.Errorf("no entity hint for a non-entity token; got: %s", h)
	}
}
