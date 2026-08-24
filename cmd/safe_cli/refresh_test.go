package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ejc3/safe_cli/internal/client"
	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

func refreshServer(t *testing.T, path, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
}

// The refresh renews the ONLINE token, so the stored set must carry an online
// refresh_token; the offline entry is the durable anchor carried forward.
func oldTokens() *tokenstore.TokenSet {
	return &tokenstore.TokenSet{
		MDN:     "5551234567",
		AppUUID: "old-uuid",
		Tokens: []tokenstore.Token{
			{FriscoTokenType: "online", IDToken: "OLDON", RefreshToken: "OLD_ON_RT"},
			{FriscoTokenType: "offline", IDToken: "OLDID", RefreshToken: "OLD_RT"},
		},
	}
}

func tokenEndpoint(d *descriptor.Descriptor) string { return d.Auth.Endpoints["user_auth_token"] }

func TestRefreshTokensCarriesIdentityAndParses(t *testing.T) {
	d, _ := descriptor.Default()
	tp := tokenEndpoint(d)
	srv := refreshServer(t, tp, `{"tokens":[{"frisco_token_type":"online","id_token":"NEWID","refresh_token":"RTon","expires_in":1800},{"frisco_token_type":"offline","id_token":"OFF2","refresh_token":"RToff2","expires_in":86400}]}`)
	defer srv.Close()
	cl := client.New("")
	cl.BaseURL = srv.URL

	ts, err := refreshTokens(context.Background(), cl, tp, "CID", "vsfapp://x/signin", oldTokens(), "app-uuid")
	if err != nil {
		t.Fatalf("refreshTokens: %v", err)
	}
	if id, ok := ts.IDToken(); !ok || id != "NEWID" {
		t.Errorf("id_token = %q", id)
	}
	// identity carried from the old set (response omitted them)
	if ts.MDN != "5551234567" || ts.AppUUID != "old-uuid" {
		t.Errorf("mdn=%q appuuid=%q", ts.MDN, ts.AppUUID)
	}
	if off, _ := ts.Offline(); off.RefreshToken != "RToff2" {
		t.Errorf("offline refresh = %q, want the fresh RToff2", off.RefreshToken)
	}
}

// If the refresh response omits an offline token, the old one is preserved so the next
// refresh still has a durable credential.
func TestRefreshTokensPreservesOldOffline(t *testing.T) {
	d, _ := descriptor.Default()
	tp := tokenEndpoint(d)
	srv := refreshServer(t, tp, `{"tokens":[{"frisco_token_type":"online","id_token":"NEWID","refresh_token":"RTon","expires_in":1800}]}`)
	defer srv.Close()
	cl := client.New("")
	cl.BaseURL = srv.URL

	ts, err := refreshTokens(context.Background(), cl, tp, "CID", "vsfapp://x/signin", oldTokens(), "U")
	if err != nil {
		t.Fatalf("refreshTokens: %v", err)
	}
	off, ok := ts.Offline()
	if !ok || off.RefreshToken != "OLD_RT" {
		t.Errorf("offline preserved = %q ok=%v, want OLD_RT", off.RefreshToken, ok)
	}
}

// A set with no refresh_token at all (neither online nor offline) can't be refreshed —
// it must error rather than silently do nothing.
func TestRefreshTokensRequiresSomeRefresh(t *testing.T) {
	old := &tokenstore.TokenSet{Tokens: []tokenstore.Token{{FriscoTokenType: "offline", IDToken: "X"}}} // no RefreshToken
	if _, err := refreshTokens(context.Background(), client.New(""), "/x", "C", "R", old, "U"); err == nil {
		t.Error("want error when there is no stored refresh_token")
	}
}

func TestWriteRefreshResult(t *testing.T) {
	ts := &tokenstore.TokenSet{MDN: "5551234567"}
	var jb strings.Builder
	if err := writeRefreshResult(&jb, true, ts); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(jb.String()), &m); err != nil || m["status"] != "ok" || m["mdn"] != "5551234567" {
		t.Errorf("json result = %q (%v)", jb.String(), err)
	}
	var pb strings.Builder
	if err := writeRefreshResult(&pb, false, ts); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.TrimSpace(pb.String()), "{") {
		t.Errorf("non-json result should be a human line: %q", pb.String())
	}
}

// If the refresh response returns an offline entry WITHOUT a refresh_token, the old
// refresh_token must be carried into it — else the next refresh has no credential.
func TestRefreshTokensPreservesWhenOfflineRefreshEmpty(t *testing.T) {
	d, _ := descriptor.Default()
	tp := tokenEndpoint(d)
	srv := refreshServer(t, tp, `{"tokens":[{"frisco_token_type":"online","id_token":"NEWID","refresh_token":"RTon","expires_in":1800},{"frisco_token_type":"offline","id_token":"OFF2","expires_in":86400}]}`)
	defer srv.Close()
	cl := client.New("")
	cl.BaseURL = srv.URL

	ts, err := refreshTokens(context.Background(), cl, tp, "CID", "vsfapp://x/signin", oldTokens(), "U")
	if err != nil {
		t.Fatalf("refreshTokens: %v", err)
	}
	off, ok := ts.Offline()
	if !ok || off.RefreshToken != "OLD_RT" {
		t.Errorf("offline refresh_token = %q ok=%v, want OLD_RT carried over", off.RefreshToken, ok)
	}
}

// When the stored set has only the durable OFFLINE refresh_token (no online) — e.g. an
// imported offline-only response — the refresh must fall back to it (friscoType
// "offline") instead of forcing a full re-login. (Regression guard: PR #17 built refresh
// on the offline token; the token-contract fix must not drop that recovery path.)
func TestRefreshTokensFallsBackToOffline(t *testing.T) {
	d, _ := descriptor.Default()
	tp := tokenEndpoint(d)
	var gotFrisco, gotRT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tp {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var body struct {
			FriscoTokenType string `json:"friscoTokenType"`
			RefreshToken    string `json:"refreshToken"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotFrisco, gotRT = body.FriscoTokenType, body.RefreshToken
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tokens":[{"frisco_token_type":"offline","id_token":"NEWOFF","refresh_token":"RToff3","expires_in":86400}]}`))
	}))
	defer srv.Close()
	cl := client.New("")
	cl.BaseURL = srv.URL

	old := &tokenstore.TokenSet{
		MDN: "5551234567", AppUUID: "u",
		Tokens: []tokenstore.Token{{FriscoTokenType: "offline", IDToken: "OLDOFF", RefreshToken: "OFF_RT"}},
	}
	ts, err := refreshTokens(context.Background(), cl, tp, "CID", "vsfapp://x/signin", old, "u")
	if err != nil {
		t.Fatalf("refreshTokens: %v", err)
	}
	if gotFrisco != "offline" || gotRT != "OFF_RT" {
		t.Errorf("request friscoTokenType=%q refreshToken=%q, want offline/OFF_RT", gotFrisco, gotRT)
	}
	if id, ok := ts.IDToken(); !ok || id != "NEWOFF" {
		t.Errorf("id_token = %q, want NEWOFF", id)
	}
}
