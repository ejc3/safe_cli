package tokenstore

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeJWT builds an unsigned JWT with the given claims (header.payload.).
func makeJWT(claims map[string]any) string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	pj, _ := json.Marshal(claims)
	return hdr + "." + base64.RawURLEncoding.EncodeToString(pj) + "."
}

func TestExpiredPrefersJWTExp(t *testing.T) {
	past := Token{IDToken: makeJWT(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})}
	if !past.Expired(0) {
		t.Error("token whose JWT exp is in the past should be expired")
	}
	// A stale ObtainedAt+ExpiresIn must not override a still-valid JWT exp.
	fut := Token{
		IDToken:    makeJWT(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}),
		ObtainedAt: 1, ExpiresIn: 1,
	}
	if fut.Expired(0) {
		t.Error("token whose JWT exp is in the future should not be expired")
	}
}

func TestSaveStampsFromJWTIatNotImportTime(t *testing.T) {
	iat := time.Now().Add(-2 * time.Hour).Unix()
	st := &Store{Path: filepath.Join(t.TempDir(), "t.json")}
	ts := &TokenSet{Tokens: []Token{{IDToken: makeJWT(map[string]any{"iat": iat}), FriscoTokenType: "online"}}}
	if err := st.Save(ts, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Tokens[0].ObtainedAt != iat {
		t.Errorf("ObtainedAt = %d, want JWT iat %d (not import time)", got.Tokens[0].ObtainedAt, iat)
	}
}

func sample() *TokenSet {
	return &TokenSet{
		MDN: "5551234567",
		Tokens: []Token{
			{IDToken: "online-id", AccessToken: "acc", RefreshToken: "online-rt", FriscoTokenType: "online", ExpiresIn: 1800},
			{IDToken: "offline-id", RefreshToken: "offline-rt", FriscoTokenType: "offline", ExpiresIn: 86400},
		},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	st := &Store{Path: filepath.Join(t.TempDir(), "sub", "tokens.json")}
	if err := st.Save(sample(), time.Now()); err != nil {
		t.Fatal(err)
	}
	// File must be 0600.
	fi, err := os.Stat(st.Path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file perm = %o, want 600", perm)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.MDN != "5551234567" || len(got.Tokens) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Tokens[0].ObtainedAt == 0 {
		t.Error("ObtainedAt should have been stamped on save")
	}
}

func TestOnlineOfflineAndIDToken(t *testing.T) {
	ts := sample()
	on, ok := ts.Online()
	if !ok || on.RefreshToken != "online-rt" {
		t.Errorf("Online() = %+v, %v", on, ok)
	}
	off, ok := ts.Offline()
	if !ok || off.RefreshToken != "offline-rt" {
		t.Errorf("Offline() = %+v, %v", off, ok)
	}
	idt, ok := ts.IDToken()
	if !ok || idt != "online-id" {
		t.Errorf("IDToken() = %q, %v; want online-id", idt, ok)
	}
}

func TestExpired(t *testing.T) {
	old := Token{ObtainedAt: time.Now().Add(-40 * time.Minute).Unix(), ExpiresIn: 1800}
	if !old.Expired(0) {
		t.Error("token obtained 40m ago with 30m TTL should be expired")
	}
	fresh := Token{ObtainedAt: time.Now().Unix(), ExpiresIn: 1800}
	if fresh.Expired(0) {
		t.Error("just-obtained token should not be expired")
	}
	// Unknown timing must not be reported as expired.
	if (Token{}).Expired(0) {
		t.Error("token with unknown timing should not be reported expired")
	}
}

func TestLoadMissing(t *testing.T) {
	st := &Store{Path: filepath.Join(t.TempDir(), "nope.json")}
	if _, err := st.Load(); !os.IsNotExist(err) {
		t.Errorf("Load of missing file = %v, want IsNotExist", err)
	}
	if err := st.Delete(); err != nil {
		t.Errorf("Delete of missing file should be nil, got %v", err)
	}
}
