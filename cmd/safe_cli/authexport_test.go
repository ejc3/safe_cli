package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// TestAuthExportImportRoundTrip pins the "move auth to another device" contract: `auth
// export` writes the stored bundle (durable offline refresh token + app_uuid), and `auth
// import` on a fresh config home restores an equal token set — so the second device can
// `auth refresh` without repeating the assisted browser login.
func TestAuthExportImportRoundTrip(t *testing.T) {
	// Device A: a config home holding a logged-in token set.
	homeA := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", homeA)
	stA, err := tokenstore.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	orig := &tokenstore.TokenSet{
		MDN:     "5551234567",
		AppUUID: "11111111-2222-4333-8444-555555555555",
		Tokens: []tokenstore.Token{
			{IDToken: "online-id", RefreshToken: "online-rt", FriscoTokenType: "online", ExpiresIn: 1800},             // #nosec G101 -- synthetic test fixture, not a real credential
			{IDToken: "offline-id", RefreshToken: "offline-rt-DURABLE", FriscoTokenType: "offline", ExpiresIn: 86400}, // #nosec G101 -- synthetic test fixture, not a real credential
		},
	}
	if err := stA.Save(orig, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Export to stdout: the raw bundle must round-trip through import.
	var buf bytes.Buffer
	rc := &runContext{G: &Globals{}, Out: &buf}
	if err := (&authExportCmd{}).Run(rc); err != nil {
		t.Fatalf("export: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("export produced no output")
	}

	// Device B: a different config home. Import the bundle from a file.
	homeB := t.TempDir()
	bundle := filepath.Join(homeB, "bundle.json")
	if err := os.WriteFile(bundle, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", homeB)
	var ibuf bytes.Buffer
	if err := (&authImportCmd{File: bundle}).Run(&runContext{G: &Globals{}, Out: &ibuf}); err != nil {
		t.Fatalf("import: %v", err)
	}

	stB, err := tokenstore.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	got, err := stB.Load()
	if err != nil {
		t.Fatal(err)
	}
	// The durable offline refresh token and the app_uuid are what let device B refresh.
	var offlineRT string
	for _, tok := range got.Tokens {
		if tok.FriscoTokenType == "offline" {
			offlineRT = tok.RefreshToken
		}
	}
	if offlineRT != "offline-rt-DURABLE" {
		t.Errorf("offline refresh token did not round-trip: got %q", offlineRT)
	}
	if got.AppUUID != orig.AppUUID {
		t.Errorf("app_uuid did not round-trip: got %q want %q", got.AppUUID, orig.AppUUID)
	}
	if got.MDN != orig.MDN {
		t.Errorf("phone did not round-trip: got %q want %q", got.MDN, orig.MDN)
	}
}

// TestAuthExportNoTokensErrors: exporting with an empty store is a clear error, not an
// empty bundle a user might mistake for a valid backup.
func TestAuthExportNoTokensErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := (&authExportCmd{}).Run(&runContext{G: &Globals{}, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("export with no stored tokens must error")
	}
}

// TestAuthExportFileIsSecret: the bundle holds a durable refresh token, so --file must land
// as a 0600 regular file even when the target already exists with loose permissions, and
// must not be written through a pre-existing symlink (which could point somewhere shared).
func TestAuthExportFileIsSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st, err := tokenstore.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(&tokenstore.TokenSet{AppUUID: "u", Tokens: []tokenstore.Token{{RefreshToken: "rt", FriscoTokenType: "offline"}}}, time.Now()); err != nil { // #nosec G101 -- synthetic test fixture
		t.Fatal(err)
	}
	dir := t.TempDir()

	// Pre-existing world-readable file at the target: export must tighten it to 0600.
	loose := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(loose, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&authExportCmd{File: loose}).Run(&runContext{G: &Globals{}, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("export over loose file: %v", err)
	}
	fi, err := os.Lstat(loose)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("exported bundle perm = %o, want 600", perm)
	}

	// Pre-existing symlink at the target: export must not write through it to the link's
	// destination (the destination must stay untouched).
	dest := filepath.Join(dir, "dest.txt")
	if err := os.WriteFile(dest, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(dest, link); err != nil {
		t.Fatal(err)
	}
	if err := (&authExportCmd{File: link}).Run(&runContext{G: &Globals{}, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("export over symlink: %v", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "keep" {
		t.Errorf("export wrote through the symlink to its destination: dest=%q", string(b))
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("target should be a fresh regular file, not the symlink (err=%v, mode=%v)", err, fi.Mode())
	}
}
