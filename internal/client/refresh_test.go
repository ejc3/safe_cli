package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The refresh hits the same token endpoint as the exchange (confirmed live), not the
// old /v6/deviceauth/refreshtoken.
const userAuthPath = "/auth/frisco/frisco-iam-device-auth/v7/user/auth/token"

func TestRefresh(t *testing.T) {
	var gotBody map[string]string
	var gotSig, gotPath, gotSrc, gotTrace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSig = r.Header.Get("x-signature")
		gotSrc = r.Header.Get("x-source-app")
		gotTrace = r.Header.Get("x-trace-transaction-id")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tokens":[` +
			`{"frisco_token_type":"online","id_token":"NEWID","access_token":"AC","refresh_token":"RTon2","expires_in":1800},` +
			`{"frisco_token_type":"offline","id_token":"OFFID","refresh_token":"RToff2","expires_in":86400}]}`))
	}))
	defer srv.Close()

	c := New("")
	c.BaseURL = srv.URL
	ts, err := c.Refresh(context.Background(), RefreshRequest{
		Path: userAuthPath, RefreshToken: "RTon", ClientID: "CID", AppUUID: daAppUUID,
		RedirectURI: "vsfapp://x/signin", FriscoType: "online",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if gotPath != userAuthPath {
		t.Errorf("path = %q, want %q", gotPath, userAuthPath)
	}
	// The refresh is UNSIGNED (it uses the token endpoint, not the signed device-auth
	// route) — the old signed contract must not come back.
	if gotSig != "" {
		t.Errorf("refresh must be unsigned; got x-signature %q", gotSig)
	}
	if gotSrc != "AndroidMAPP" {
		t.Errorf("x-source-app = %q, want AndroidMAPP", gotSrc)
	}
	// The token endpoint rejects a refresh without x-trace-transaction-id (400) — it
	// must be present on every token-endpoint request.
	if gotTrace == "" {
		t.Error("missing x-trace-transaction-id header; the token endpoint requires it")
	}
	// camelCase body: grantType=refresh_token, the refreshToken echoed, and the
	// friscoTokenType matching the refresh token's type — with no snake_case leftovers.
	if gotBody["grantType"] != "refresh_token" || gotBody["refreshToken"] != "RTon" ||
		gotBody["clientId"] != "CID" || gotBody["friscoTokenType"] != "online" ||
		gotBody["appUuid"] != daAppUUID || gotBody["identityProvider"] != "vz-am-provider" {
		t.Errorf("body = %v", gotBody)
	}
	if _, snake := gotBody["grant_type"]; snake {
		t.Errorf("body must be camelCase, found snake_case grant_type: %v", gotBody)
	}
	if id, ok := ts.IDToken(); !ok || id != "NEWID" {
		t.Errorf("id_token = %q ok=%v, want NEWID", id, ok)
	}
}

func TestRefreshRequiresRefreshToken(t *testing.T) {
	c := New("")
	if _, err := c.Refresh(context.Background(), RefreshRequest{Path: userAuthPath, ClientID: "C", AppUUID: "U"}); err == nil {
		t.Error("want error without a refresh_token")
	}
}

func TestRefreshNoIDToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tokens":[]}`))
	}))
	defer srv.Close()
	c := New("")
	c.BaseURL = srv.URL
	if _, err := c.Refresh(context.Background(), RefreshRequest{Path: userAuthPath, RefreshToken: "RT", ClientID: "C", AppUUID: "U"}); err == nil {
		t.Error("want error when the refresh response carries no id_token")
	}
}
