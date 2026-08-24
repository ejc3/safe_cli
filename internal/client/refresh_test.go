package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const refreshPath = "/auth/frisco/frisco-iam-device-auth/v6/deviceauth/refreshtoken"

func TestRefresh(t *testing.T) {
	var gotBody map[string]string
	var gotSig, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSig = r.Header.Get("x-signature")
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
		Path: refreshPath, RefreshToken: "RToff", ClientID: "CID", AppUUID: daAppUUID, Key: daKey,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if gotPath != refreshPath {
		t.Errorf("path = %q", gotPath)
	}
	if len(gotSig) != 64 {
		t.Errorf("refresh must be signed; x-signature len = %d", len(gotSig))
	}
	if gotBody["grant_type"] != "refresh_token" || gotBody["code"] != "RToff" ||
		gotBody["client_id"] != "CID" || gotBody["frisco_token_type"] != "offline" {
		t.Errorf("body = %v", gotBody)
	}
	if id, ok := ts.IDToken(); !ok || id != "NEWID" {
		t.Errorf("id_token = %q ok=%v, want NEWID", id, ok)
	}
}

func TestRefreshRequiresRefreshToken(t *testing.T) {
	c := New("")
	if _, err := c.Refresh(context.Background(), RefreshRequest{Path: refreshPath, ClientID: "C", AppUUID: "U", Key: "K"}); err == nil {
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
	if _, err := c.Refresh(context.Background(), RefreshRequest{Path: refreshPath, RefreshToken: "RT", ClientID: "C", AppUUID: "U", Key: "K"}); err == nil {
		t.Error("want error when the refresh response carries no id_token")
	}
}
