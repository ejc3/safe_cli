package main

// containsStr reports whether xs contains x (the descriptor package keeps its own
// unexported helper; the engine needs one here for enum checks).
func containsStr(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
