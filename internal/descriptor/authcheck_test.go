package descriptor

import (
	"os"
	"strings"
	"testing"
)

// TestAuthEndpointsAreDiscovered cross-checks every auth endpoint in the
// descriptor against the catalog harvested from the app. It guards the class of
// bug Codex found on PR #1: the descriptor pointed `authorize` at
// `/frisco/.../v6/oauth2/authorize`, a route that does not exist — the real v6
// authorize is under `/nsauth/`, and the observed PKCE route is v5.
func TestAuthEndpointsAreDiscovered(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	// Tests run with CWD = this package dir; the catalog lives at repo docs/.
	raw, err := os.ReadFile("../../docs/discovered-endpoints.txt")
	if err != nil {
		t.Fatalf("read discovered-endpoints catalog: %v", err)
	}
	discovered := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		discovered[line] = true
	}
	if len(discovered) < 100 {
		t.Fatalf("catalog looks truncated: only %d entries", len(discovered))
	}
	for name, p := range d.Auth.Endpoints {
		key := strings.TrimPrefix(p, "/")
		if !discovered[key] {
			t.Errorf("auth endpoint %q = %q is not a route in the discovered catalog", name, p)
		}
	}
}
