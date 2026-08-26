package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ejc3/safe_cli/internal/descriptor"
	"github.com/ejc3/safe_cli/internal/tokenstore"
)

// The durability guarantee for the working login: once authenticated, a call whose id_token
// has expired must not fail — authedRequest transparently refreshes with the stored
// refresh_token and retries, and persists the fresh set. This exercises the real path (API
// 401 → refresh → retry → 200) against a mock backend. It fails without the on-401 refresh
// (the first 401 would be returned as-is).
func TestAuthedRequestRefreshesOn401(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	refreshPath := d.Auth.Endpoints["user_auth_token"]

	var refreshHits, apiHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == refreshPath {
			refreshHits++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tokens":[` +
				`{"frisco_token_type":"online","id_token":"NEWID","refresh_token":"RT2","expires_in":1800},` +
				`{"frisco_token_type":"offline","id_token":"OFF","refresh_token":"RTO","expires_in":86400}]}`))
			return
		}
		apiHits++
		if r.Header.Get("Authorization") == "NEWID" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"expired id_token"}`))
	}))
	defer srv.Close()

	d.BaseURL = srv.URL
	st := &tokenstore.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}
	ts := &tokenstore.TokenSet{
		AppUUID: "00000000-0000-4000-8000-000000000000",
		Tokens: []tokenstore.Token{
			{IDToken: "OLDID", RefreshToken: "RT", FriscoTokenType: "online", ExpiresIn: 1800},
		},
	}
	if err := st.Save(ts, time.Now()); err != nil {
		t.Fatal(err)
	}

	rc := &runContext{D: d, G: &Globals{}, Out: &strings.Builder{}}
	do := authedRequest(rc, st, ts)
	resp, err := do(context.Background(), "GET", "/api/anything", nil, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200 after transparent refresh", resp.Status)
	}
	if refreshHits != 1 {
		t.Errorf("refresh hits = %d, want exactly 1", refreshHits)
	}
	if apiHits != 2 {
		t.Errorf("api hits = %d, want 2 (401, then a retry with the fresh token)", apiHits)
	}
	// The fresh token set must be persisted so subsequent runs start authenticated.
	reloaded, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if idt, _ := reloaded.IDToken(); idt != "NEWID" {
		t.Errorf("persisted id_token = %q, want NEWID", idt)
	}
	if off, ok := reloaded.Offline(); !ok || off.RefreshToken != "RTO" {
		t.Errorf("durable offline refresh_token not persisted: %+v ok=%v", off, ok)
	}
}

// When the refresh itself fails (no way to get a fresh token), the original 401 is surfaced
// rather than masked — the caller sees the real auth failure.
func TestAuthedRequestSurfaces401WhenRefreshFails(t *testing.T) {
	d, err := descriptor.Default()
	if err != nil {
		t.Fatal(err)
	}
	refreshPath := d.Auth.Endpoints["user_auth_token"]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == refreshPath {
			w.WriteHeader(http.StatusBadRequest) // refresh rejected
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"expired"}`))
	}))
	defer srv.Close()

	d.BaseURL = srv.URL
	st := &tokenstore.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}
	ts := &tokenstore.TokenSet{
		AppUUID: "00000000-0000-4000-8000-000000000000",
		Tokens:  []tokenstore.Token{{IDToken: "OLDID", RefreshToken: "RT", FriscoTokenType: "online"}},
	}
	rc := &runContext{D: d, G: &Globals{}, Out: &strings.Builder{}}
	resp, err := authedRequest(rc, st, ts)(context.Background(), "GET", "/api/anything", nil, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.Status != http.StatusUnauthorized {
		t.Errorf("status = %d, want the original 401 surfaced when refresh fails", resp.Status)
	}
}
