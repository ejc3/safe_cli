package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/descriptor/gen"
)

// The committed tree must be exactly what the descriptor generates: editing a cli block
// without running `go generate ./cmd/safe_cli` fails here (the drift check the design
// calls for), and no one can hand-edit the generated file.
func TestGeneratedTreeIsCurrent(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	want, err := gen.Source(d)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	got, err := os.ReadFile("zz_generated_tree.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("zz_generated_tree.go is stale: run `go generate ./cmd/safe_cli` and commit the result")
	}
}

// The generated help is the public contract an agent reads (docs/CLI-DESIGN.md §9): the
// area lists its verbs, and a verb's help states the enum, the default, and the PAIRED
// prerequisite — the three things the design probe found missing before.
func TestGeneratedHelpIsDiscoverable(t *testing.T) {
	out := helpFor(t, "pause-internet", "pause", "--help")
	for _, want := range []string{
		"Pause the child's internet now",
		"Prerequisite: The child's phone must be PAIRED",
		"--allow-unpaired",
		"--child=CHILD",
		"30m|1h|2h|4h|until-morning",
		"(default: 30m)",
		"--indefinite",
		"--dry-run",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pause --help lacks %q:\n%s", want, out)
		}
	}
	top := helpFor(t, "--help")
	if !strings.Contains(top, "pause-internet") {
		t.Errorf("top-level --help must list the area:\n%s", top)
	}
	areaHelp := helpFor(t, "pause-internet", "--help")
	if !strings.Contains(areaHelp, "Pause or resume a child's internet") {
		t.Errorf("pause-internet --help must carry the area's help:\n%s", areaHelp)
	}
	for _, verb := range []string{"status", "pause", "resume"} {
		if !strings.Contains(areaHelp, verb) {
			t.Errorf("pause-internet --help lacks verb %q:\n%s", verb, areaHelp)
		}
	}
}
