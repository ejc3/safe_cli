package main

import (
	"fmt"
	"strings"

	"github.com/ejc3/safe_cli/internal/descriptor"
)

// entityCommandHint returns an actionable message when the first CLI argument is a known
// entity name that kong rejected as an unknown command — the natural mistake of treating an
// entity like the generated area commands (filter, apps, …). It points at call/describe
// instead of leaving the raw "unexpected argument" error (issue #83). It returns "" when the
// parse succeeded, the token is not an entity, or the failure is unrelated (a real command
// missing a flag), so kong's own error stands.
func entityCommandHint(d *descriptor.Descriptor, args []string, parseErr error) string {
	if parseErr == nil || len(args) == 0 {
		return ""
	}
	name := args[0]
	if _, ok := d.Entity(name); !ok {
		return ""
	}
	// Only when kong rejected this very token as an unexpected argument (an entity used as a
	// command), not when a valid command failed for another reason.
	if !strings.Contains(parseErr.Error(), "unexpected argument "+name) {
		return ""
	}
	return fmt.Sprintf("safe_cli: %s is an entity, not a command. "+
		"Use `safe_cli call %s <op>` to run an operation, or `safe_cli describe %s` to list its operations.",
		name, name, name)
}
