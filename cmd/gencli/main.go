// Command gencli writes cmd/safe_cli/zz_generated_tree.go from the embedded descriptor.
// It is run by `go generate` (see cmd/safe_cli/generate.go); the committed file is checked
// against a fresh render by a test, so the tree can never drift from the descriptor.
package main

import (
	"fmt"
	"os"

	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/descriptor/gen"
)

func main() {
	out := "cmd/safe_cli/zz_generated_tree.go"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	d, err := descriptor.Default()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src, err := gen.Source(d)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, src, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
