package main

// The subcommand tree is generated from the descriptor's cli blocks; regenerate after
// editing verizon_family.json. TestGeneratedTreeIsCurrent fails when the file drifts.
//go:generate go run ../gencli zz_generated_tree.go

// deref returns the string a generated --child flag holds, or "" when it was not given.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
