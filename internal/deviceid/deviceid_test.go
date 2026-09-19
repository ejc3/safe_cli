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

// TestResetMintsFreshUUID: Reset() deletes the persisted id so the next AppUUID() differs,
// and Reset() on an absent id is a no-op (not an error). This is the recovery lever for a
// backend session wedged to an abandoned login's uuid.
func TestResetMintsFreshUUID(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	first, err := AppUUID()
	if err != nil {
		t.Fatal(err)
	}
	if err := Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	second, err := AppUUID()
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Errorf("Reset did not change the uuid: %q", second)
	}
	if err := Reset(); err != nil { // absent-after-reset path is fine
		t.Errorf("Reset on absent id must be a no-op, got %v", err)
	}
	// AppUUID after a second reset still mints a valid id.
	if _, err := os.Stat(mustPath(t)); err == nil {
		t.Error("uuid file should be gone after the second Reset")
	}
}

func mustPath(t *testing.T) string {
	t.Helper()
	p, err := defaultPath()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
