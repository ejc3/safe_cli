package deviceid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppUUIDStableValidPersisted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	first, err := AppUUID()
	if err != nil {
		t.Fatalf("AppUUID: %v", err)
	}
	if !uuidRE.MatchString(first) {
		t.Fatalf("not a uuid: %q", first)
	}
	second, err := AppUUID()
	if err != nil {
		t.Fatalf("AppUUID (2): %v", err)
	}
	if first != second {
		t.Fatalf("not stable: %q != %q", first, second)
	}
	p := filepath.Join(dir, "safe_cli", "appuuid")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("expected persisted file at %s: %v", p, err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}
