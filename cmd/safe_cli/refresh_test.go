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

func oldTokens() *tokenstore.TokenSet {
	return &tokenstore.TokenSet{
		MDN:     "5551234567",
		AppUUID: "old-uuid",
		Tokens:  []tokenstore.Token{{FriscoTokenType: "offline", IDToken: "OLDID", RefreshToken: "OLD_RT"}},
	}
}

func TestRefreshTokensCarriesIdentityAndParses(t *testing.T) {
	d, _ := descriptor.Default()
	rp := d.Auth.Endpoints["refresh"]
	srv := refreshServer(t, rp, `{"tokens":[{"frisco_token_type":"online","id_token":"NEWID","refresh_token":"RTon","expires_in":1800},{"frisco_token_type":"offline","id_token":"OFF2","refresh_token":"RToff2","expires_in":86400}]}`)
	defer srv.Close()
	cl := client.New("")
	cl.BaseURL = srv.URL

	ts, err := refreshTokens(context.Background(), cl, rp, "CID", oldTokens(),
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "app-uuid")
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
	rp := d.Auth.Endpoints["refresh"]
	srv := refreshServer(t, rp, `{"tokens":[{"frisco_token_type":"online","id_token":"NEWID","refresh_token":"RTon","expires_in":1800}]}`)
	defer srv.Close()
	cl := client.New("")
	cl.BaseURL = srv.URL

	ts, err := refreshTokens(context.Background(), cl, rp, "CID", oldTokens(), "K", "U")
	if err != nil {
		t.Fatalf("refreshTokens: %v", err)
	}
	off, ok := ts.Offline()
	if !ok || off.RefreshToken != "OLD_RT" {
		t.Errorf("offline preserved = %q ok=%v, want OLD_RT", off.RefreshToken, ok)
	}
}

func TestRefreshTokensNoOfflineToStart(t *testing.T) {
	old := &tokenstore.TokenSet{Tokens: []tokenstore.Token{{FriscoTokenType: "online", IDToken: "X"}}}
	if _, err := refreshTokens(context.Background(), client.New(""), "/x", "C", old, "K", "U"); err == nil {
		t.Error("want error when there is no stored offline refresh_token")
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
